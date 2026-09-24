package api

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
	"github.com/azurebrasil/argocd-vpa-updater/internal/eligibility"
	"github.com/azurebrasil/argocd-vpa-updater/internal/idempotency"
	"github.com/azurebrasil/argocd-vpa-updater/internal/resolver"
	"github.com/azurebrasil/argocd-vpa-updater/internal/statestore"
	"github.com/azurebrasil/argocd-vpa-updater/internal/vparecommendation"
	"github.com/azurebrasil/argocd-vpa-updater/internal/workloadresources"
)

// ErrRecommendationNotFound is returned by SelectRecommendation when no
// opted-in, valid VPA has a recommendation for the requested container.
var ErrRecommendationNotFound = errors.New("api: recommendation not found")

// ErrRecommendationNotEligible is returned by SelectRecommendation when the
// recommendation exists but doesn't clear the eligibility bar (see
// internal/eligibility) -- the server never trusts the dashboard alone to
// have hidden the "Accept suggestion" button.
var ErrRecommendationNotEligible = errors.New("api: recommendation is not eligible for selection")

// ErrInvalidSelectRequest is returned when a select request carries an
// out-of-range option (e.g. limitHeadroomPercent >= 100).
var ErrInvalidSelectRequest = errors.New("api: invalid select request")

// Service composes the read-side contracts (VpaRecommendationReader,
// GitOpsTargetResolver, workloadresources.Reader, StateStore) into the data
// the dashboard needs. It performs no write-back of its own in this phase
// -- selection/apply endpoints are not wired up yet (see the write-back
// seam in internal/gitwriteback).
type Service struct {
	Reader         vparecommendation.VpaRecommendationReader
	Resolver       resolver.GitOpsTargetResolver
	WorkloadReader workloadresources.Reader
	State          statestore.StateStore

	// Now is overridable for deterministic tests; defaults to time.Now.
	Now func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// listRecommendationsConcurrency bounds how many containers' evaluate()
// calls (each now a live Kubernetes API read, see internal/workloadresources)
// run at once, so a dashboard load with many opted-in VPAs pays roughly one
// read's latency overall instead of the sum of all of them, without firing
// an unbounded number of requests at the API server at once.
const listRecommendationsConcurrency = 16

// ListRecommendations returns one RecommendationDTO per container the VPA's
// VpaGitOpsBinding configures (a binding may configure more than one
// container).
func (s *Service) ListRecommendations(ctx context.Context) ([]RecommendationDTO, error) {
	vpas, err := s.Reader.ListOptedIn(ctx)
	if err != nil {
		return nil, fmt.Errorf("api: list opted-in VPAs: %w", err)
	}
	// ListOptedIn's order is not guaranteed stable across calls (the default
	// reader's backing Cache is a Go map, and Go intentionally randomizes map
	// iteration order) -- without this, the dashboard's row order would
	// shuffle on every poll for any VPAs the client's own sort treats as tied
	// (e.g. same namespace), which is disruptive to look at even though the
	// data itself hasn't changed.
	sort.Slice(vpas, func(i, j int) bool {
		if vpas[i].Namespace != vpas[j].Namespace {
			return vpas[i].Namespace < vpas[j].Namespace
		}
		return vpas[i].Name < vpas[j].Name
	})

	doc, _, err := s.State.Get(ctx)
	if err != nil {
		// Degrade gracefully: the dashboard is still useful without status
		// history, and a StateStore outage must not take down the read path.
		doc = domain.NewEmptyStateDocument()
	}

	// Each slot is either already resolved (invalid VPA, no recommendation
	// yet) or still needs evaluate() to run; build() runs the latter
	// concurrently, writing into its own index so output order matches the
	// order VPAs/containers were encountered regardless of completion order.
	type slot struct {
		dto   RecommendationDTO
		build func() RecommendationDTO
	}
	var slots []slot
	for _, vpa := range vpas {
		if !vpa.Valid {
			for _, dto := range s.invalidVPADTOs(vpa) {
				slots = append(slots, slot{dto: dto})
			}
			continue
		}
		for _, cc := range vpa.Binding.Containers {
			cr, found := vpa.ContainerByName(cc.ContainerName)
			if !found {
				dto := s.baseDTO(vpa, domain.ContainerRecommendation{ContainerName: cc.ContainerName}, doc)
				dto.CurrentValueError = fmt.Sprintf("VPA has no recommendation yet for container %q", cc.ContainerName)
				slots = append(slots, slot{dto: dto})
				continue
			}
			vpa, cr := vpa, cr
			slots = append(slots, slot{build: func() RecommendationDTO { return s.buildDTO(ctx, vpa, cr, doc) }})
		}
	}

	items := make([]RecommendationDTO, len(slots))
	var wg sync.WaitGroup
	sem := make(chan struct{}, listRecommendationsConcurrency)
	for i, sl := range slots {
		if sl.build == nil {
			items[i] = sl.dto
			continue
		}
		i, build := i, sl.build
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			items[i] = build()
		}()
	}
	wg.Wait()

	return items, nil
}

// GetRecommendation returns the single RecommendationDTO for one
// (namespace, vpaName, containerName), or false if not found among opted-in
// VPAs.
func (s *Service) GetRecommendation(ctx context.Context, namespace, vpaName, containerName string) (RecommendationDTO, bool, error) {
	items, err := s.ListRecommendations(ctx)
	if err != nil {
		return RecommendationDTO{}, false, err
	}
	for _, item := range items {
		if item.Namespace == namespace && item.VPAName == vpaName && item.ContainerName == containerName {
			return item, true, nil
		}
	}
	return RecommendationDTO{}, false, nil
}

// invalidVPADTOs returns one entry per container the binding declares (so
// each still shows up in the list with its validation errors), or a single
// generic entry if the binding itself failed validation before any
// container list could even be trusted.
func (s *Service) invalidVPADTOs(vpa domain.NormalizedVPA) []RecommendationDTO {
	base := RecommendationDTO{
		Cluster:          vpa.Cluster,
		Namespace:        vpa.Namespace,
		VPAName:          vpa.Name,
		Workload:         toWorkloadDTO(vpa.Workload),
		UpdateMode:       vpa.UpdateMode,
		Status:           "failed",
		ValidationErrors: vpa.ValidationErrors,
	}
	if len(vpa.Binding.Containers) == 0 {
		return []RecommendationDTO{base}
	}
	out := make([]RecommendationDTO, 0, len(vpa.Binding.Containers))
	for _, cc := range vpa.Binding.Containers {
		dto := base
		dto.ContainerName = cc.ContainerName
		out = append(out, dto)
	}
	return out
}

func (s *Service) baseDTO(vpa domain.NormalizedVPA, cr domain.ContainerRecommendation, doc *domain.StateDocument) RecommendationDTO {
	var ageSeconds int64
	if !vpa.RecommendationTime.IsZero() {
		ageSeconds = int64(s.now().Sub(vpa.RecommendationTime).Seconds())
	}
	return RecommendationDTO{
		Cluster:                  vpa.Cluster,
		Namespace:                vpa.Namespace,
		VPAName:                  vpa.Name,
		Workload:                 toWorkloadDTO(vpa.Workload),
		ContainerName:            cr.ContainerName,
		UpdateMode:               vpa.UpdateMode,
		RecommendedCPU:           quantityString(cr.Target.CPU),
		RecommendedMemory:        quantityString(cr.Target.Memory),
		LowerBoundCPU:            quantityString(cr.LowerBound.CPU),
		LowerBoundMemory:         quantityString(cr.LowerBound.Memory),
		UpperBoundCPU:            quantityString(cr.UpperBound.CPU),
		UpperBoundMemory:         quantityString(cr.UpperBound.Memory),
		RecommendationAgeSeconds: ageSeconds,
		Status:                   statusFor(doc, vpa.Namespace, vpa.Name, cr.ContainerName),
		Operation:                operationDTOFor(doc, vpa.Namespace, vpa.Name, cr.ContainerName),
	}
}

func operationDTOFor(doc *domain.StateDocument, namespace, vpaName, containerName string) *OperationDTO {
	op := operationFor(doc, namespace, vpaName, containerName)
	if op == nil {
		return nil
	}
	return &OperationDTO{
		Branch:       op.Branch,
		CommitSHA:    op.CommitSHA,
		PRURL:        op.PRURL,
		ErrorMessage: op.ErrorMessage,
		UpdatedAt:    op.UpdatedAt,
	}
}

func (s *Service) buildDTO(ctx context.Context, vpa domain.NormalizedVPA, cr domain.ContainerRecommendation, doc *domain.StateDocument) RecommendationDTO {
	dto := s.baseDTO(vpa, cr, doc)

	eval, err := s.evaluate(ctx, vpa, cr)
	dto.Warnings = eval.Target.Warnings
	if err != nil {
		dto.CurrentValueError = err.Error()
		return dto
	}

	dto.CurrentCPU = quantityString(eval.Current.CPU)
	dto.CurrentMemory = quantityString(eval.Current.Memory)
	dto.CurrentCPULimit = quantityString(eval.CurrentLimits.CPU)
	dto.CurrentMemoryLimit = quantityString(eval.CurrentLimits.Memory)
	dto.CPULimitConfigured = eval.Target.CPULimitKeyPath != ""
	dto.MemoryLimitConfigured = eval.Target.MemoryLimitKeyPath != ""
	dto.CPULimitRequired = limitRequired(eval.Target.CPULimitKeyPath, eval.CurrentLimits.CPU, cr.Target.CPU)
	dto.MemoryLimitRequired = limitRequired(eval.Target.MemoryLimitKeyPath, eval.CurrentLimits.Memory, cr.Target.Memory)
	dto.CPULimitExceeded = limitExceeded(eval.CurrentLimits.CPU, cr.Target.CPU)
	dto.MemoryLimitExceeded = limitExceeded(eval.CurrentLimits.Memory, cr.Target.Memory)
	dto.DeltaCPUAbsoluteMilli = eval.CPUDelta.AbsoluteMilli
	dto.DeltaCPUPercent = finitePercent(eval.CPUDelta)
	dto.DeltaMemoryAbsoluteMilli = eval.MemoryDelta.AbsoluteMilli
	dto.DeltaMemoryPercent = finitePercent(eval.MemoryDelta)
	dto.Eligible = eval.Eligibility.Eligible
	dto.EligibilityReasons = eval.Eligibility.Reasons

	dto.CPUConfigured = eval.Target.CPUKeyPath != ""
	dto.CPUEligible = eval.Eligibility.CPU.Eligible
	dto.CPUEligibilityReasons = eval.Eligibility.CPU.Reasons
	dto.MemoryConfigured = eval.Target.MemoryKeyPath != ""
	dto.MemoryEligible = eval.Eligibility.Memory.Eligible
	dto.MemoryEligibilityReasons = eval.Eligibility.Memory.Reasons

	return dto
}

// evaluation is the shared result of resolving a container's write target,
// reading its current value from the live workload, and evaluating
// eligibility -- used by both buildDTO (read-only display) and
// SelectRecommendation (which must reach the identical eligibility verdict
// the dashboard just showed the user before queuing a selection).
type evaluation struct {
	Target        domain.WriteTarget
	Current       domain.ResourceAmount
	CurrentLimits domain.ResourceAmount
	CPUDelta      eligibility.Delta
	MemoryDelta   eligibility.Delta
	Eligibility   eligibility.Result
}

func (s *Service) evaluate(ctx context.Context, vpa domain.NormalizedVPA, cr domain.ContainerRecommendation) (evaluation, error) {
	var eval evaluation

	target, err := s.Resolver.Resolve(ctx, vpa, cr.ContainerName)
	if err != nil {
		return eval, err
	}
	eval.Target = target

	values, err := s.WorkloadReader.CurrentValues(ctx, vpa.Workload, cr.ContainerName)
	if err != nil {
		return eval, fmt.Errorf("reading current values from live workload: %w", err)
	}
	current := values.Requests
	eval.Current = current
	eval.CurrentLimits = values.Limits

	eval.CPUDelta = eligibility.ComputeDelta(current.CPU, cr.Target.CPU)
	eval.MemoryDelta = eligibility.ComputeDelta(current.Memory, cr.Target.Memory)
	eval.Eligibility = eligibility.Evaluate(eligibility.Input{
		CPU:              eval.CPUDelta,
		Memory:           eval.MemoryDelta,
		ApplyCPU:         target.CPUKeyPath != "",
		ApplyMemory:      target.MemoryKeyPath != "",
		MinChangePercent: vpa.Binding.EffectiveMinChangePercent(cr.ContainerName),
	})

	return eval, nil
}

// SelectOptions is what a caller asks a selection to apply: which
// resources, and optionally how to set each one's limit (see
// domain.LimitSpec; nil leaves that limit untouched).
type SelectOptions struct {
	ApplyCPU    bool
	ApplyMemory bool
	CPULimit    *domain.LimitSpec
	MemoryLimit *domain.LimitSpec
}

func (o SelectOptions) validate() error {
	if !o.ApplyCPU && !o.ApplyMemory {
		return fmt.Errorf("%w: no resource selected", ErrRecommendationNotEligible)
	}
	if o.CPULimit != nil {
		if err := o.CPULimit.Validate(true); err != nil {
			return fmt.Errorf("%w: cpu limit: %v", ErrInvalidSelectRequest, err)
		}
	}
	if o.MemoryLimit != nil {
		if err := o.MemoryLimit.Validate(false); err != nil {
			return fmt.Errorf("%w: memory limit: %v", ErrInvalidSelectRequest, err)
		}
	}
	return nil
}

// checkAbsoluteLimit rejects an absolute limit below the recommendation it
// would sit above -- Kubernetes rejects request > limit, so queuing it would
// only fail later at write-back (or worse, at Argo CD sync).
func checkAbsoluteLimit(resourceName string, spec *domain.LimitSpec, recommended *resource.Quantity) error {
	if spec == nil || spec.Value == nil || recommended == nil || spec.Value.Cmp(*recommended) >= 0 {
		return nil
	}
	return fmt.Errorf("%w: %s limit %s is below the recommended request %s", ErrInvalidSelectRequest, resourceName, spec.Value.String(), recommended.String())
}

// limitExceeded reports whether writing recommended as the request would
// exceed the live limit (false when the workload declares none).
func limitExceeded(currentLimit, recommended *resource.Quantity) bool {
	return currentLimit != nil && recommended != nil && recommended.Cmp(*currentLimit) > 0
}

// limitRequired reports whether a new limit must be written alongside the
// request: it would exceed the live limit, and write-back manages that
// limit (limitKeyPath set). Exceeding an unmanaged limit can't be fixed by
// write-back, so it's surfaced as a warning instead (see LimitExceeded).
func limitRequired(limitKeyPath string, currentLimit, recommended *resource.Quantity) bool {
	return limitKeyPath != "" && limitExceeded(currentLimit, recommended)
}

// checkRequiredLimit rejects a selection that would leave the limit
// untouched below the new request.
func checkRequiredLimit(resourceName string, spec *domain.LimitSpec, limitKeyPath string, currentLimit, recommended *resource.Quantity) error {
	if spec != nil || !limitRequired(limitKeyPath, currentLimit, recommended) {
		return nil
	}
	return fmt.Errorf("%w: %s request %s exceeds the current limit %s: a new %s limit is required", ErrInvalidSelectRequest, resourceName, recommended.String(), currentLimit.String(), resourceName)
}

// findContainer locates the opted-in, valid VPA + container recommendation
// for (namespace, vpaName, containerName).
func (s *Service) findContainer(ctx context.Context, namespace, vpaName, containerName string) (domain.NormalizedVPA, domain.ContainerRecommendation, bool, error) {
	vpas, err := s.Reader.ListOptedIn(ctx)
	if err != nil {
		return domain.NormalizedVPA{}, domain.ContainerRecommendation{}, false, fmt.Errorf("api: list opted-in VPAs: %w", err)
	}
	for _, v := range vpas {
		if v.Namespace != namespace || v.Name != vpaName || !v.Valid {
			continue
		}
		if c, ok := v.ContainerByName(containerName); ok {
			return v, c, true, nil
		}
	}
	return domain.NormalizedVPA{}, domain.ContainerRecommendation{}, false, nil
}

// buildSelection resolves, evaluates and validates a selection for one
// container without persisting it.
//
// strict=true (the single-container select path, where the dashboard only
// ever lets a user tick a checkbox that's already configured+eligible)
// rejects a requested resource outright if it turns out not to be
// configured or individually eligible -- that can only happen if state
// changed between page load and click, and the caller should surface it as
// a normal error.
//
// strict=false (the bulk path, driven by "accept all CPU/memory/both"
// rather than a specific checkbox) instead silently drops a requested
// resource that isn't configured/eligible for this particular container,
// only failing if *nothing* requested ends up selectable -- a bulk action
// must never abort the whole batch over one container.
func (s *Service) buildSelection(ctx context.Context, vpa domain.NormalizedVPA, cr domain.ContainerRecommendation, opts SelectOptions, strict bool) (domain.PendingSelection, error) {
	applyCPU, applyMemory := opts.ApplyCPU, opts.ApplyMemory

	eval, err := s.evaluate(ctx, vpa, cr)
	if err != nil {
		return domain.PendingSelection{}, fmt.Errorf("%w: %v", ErrRecommendationNotFound, err)
	}

	wantCPU, wantMemory := applyCPU, applyMemory

	if applyCPU {
		if eval.Target.CPUKeyPath == "" {
			if strict {
				return domain.PendingSelection{}, fmt.Errorf("%w: cpu is not configured for this container", ErrRecommendationNotEligible)
			}
			wantCPU = false
		} else if !eval.Eligibility.CPU.Eligible {
			if strict {
				return domain.PendingSelection{}, fmt.Errorf("%w: cpu: %v", ErrRecommendationNotEligible, eval.Eligibility.CPU.Reasons)
			}
			wantCPU = false
		}
	}
	if applyMemory {
		if eval.Target.MemoryKeyPath == "" {
			if strict {
				return domain.PendingSelection{}, fmt.Errorf("%w: memory is not configured for this container", ErrRecommendationNotEligible)
			}
			wantMemory = false
		} else if !eval.Eligibility.Memory.Eligible {
			if strict {
				return domain.PendingSelection{}, fmt.Errorf("%w: memory: %v", ErrRecommendationNotEligible, eval.Eligibility.Memory.Reasons)
			}
			wantMemory = false
		}
	}

	if !wantCPU && !wantMemory {
		return domain.PendingSelection{}, fmt.Errorf("%w: no requested resource is configured and eligible", ErrRecommendationNotEligible)
	}

	// A limit spec only travels with the resource it belongs to.
	var cpuLimit, memoryLimit *domain.LimitSpec
	if wantCPU {
		cpuLimit = opts.CPULimit
		if err := checkAbsoluteLimit("cpu", cpuLimit, cr.Target.CPU); err != nil {
			return domain.PendingSelection{}, err
		}
		if err := checkRequiredLimit("cpu", cpuLimit, eval.Target.CPULimitKeyPath, eval.CurrentLimits.CPU, cr.Target.CPU); err != nil {
			return domain.PendingSelection{}, err
		}
	}
	if wantMemory {
		memoryLimit = opts.MemoryLimit
		if err := checkAbsoluteLimit("memory", memoryLimit, cr.Target.Memory); err != nil {
			return domain.PendingSelection{}, err
		}
		if err := checkRequiredLimit("memory", memoryLimit, eval.Target.MemoryLimitKeyPath, eval.CurrentLimits.Memory, cr.Target.Memory); err != nil {
			return domain.PendingSelection{}, err
		}
	}

	patchReq := domain.PatchRequest{
		Target:         eval.Target,
		Recommendation: cr,
		ApplyCPU:       wantCPU,
		ApplyMemory:    wantMemory,
		CPULimit:       cpuLimit,
		MemoryLimit:    memoryLimit,
	}

	return domain.PendingSelection{
		IdempotencyKey:        idempotency.Key(vpa.Namespace, vpa.Name, eval.Target, patchReq),
		VPANamespace:          vpa.Namespace,
		VPAName:               vpa.Name,
		ContainerName:         cr.ContainerName,
		SelectedAt:            s.now(),
		Target:                eval.Target,
		ApplyCPU:              wantCPU,
		ApplyMemory:           wantMemory,
		RecommendationSummary: cr.Target,
		CPULimit:              cpuLimit,
		MemoryLimit:           memoryLimit,
	}, nil
}

// persistSelections writes sels into the state document in a single
// read-modify-write, replacing (not duplicating) any earlier pending
// selection for the same (namespace, vpaName, containerName).
func (s *Service) persistSelections(ctx context.Context, sels []domain.PendingSelection) error {
	if len(sels) == 0 {
		return nil
	}
	err := s.State.Update(ctx, func(doc *domain.StateDocument) error {
		for _, sel := range sels {
			doc.PendingSelections = replaceSelection(doc.PendingSelections, sel)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("api: persist selection: %w", err)
	}
	return nil
}

func replaceSelection(list []domain.PendingSelection, sel domain.PendingSelection) []domain.PendingSelection {
	kept := list[:0]
	for _, existing := range list {
		if existing.VPANamespace == sel.VPANamespace && existing.VPAName == sel.VPAName && existing.ContainerName == sel.ContainerName {
			continue // replaced by sel, appended below
		}
		kept = append(kept, existing)
	}
	return append(kept, sel)
}

// SelectRecommendation queues the current recommendation's requested
// resource(s) for (namespace, vpaName, containerName) as a PendingSelection
// -- it does not write anything to Git itself (see internal/gitwriteback
// for that seam, not yet wired up). Re-selecting the same container
// replaces any earlier pending selection for it rather than accumulating
// duplicates.
func (s *Service) SelectRecommendation(ctx context.Context, namespace, vpaName, containerName string, opts SelectOptions) (domain.PendingSelection, error) {
	if err := opts.validate(); err != nil {
		return domain.PendingSelection{}, err
	}

	vpa, cr, found, err := s.findContainer(ctx, namespace, vpaName, containerName)
	if err != nil {
		return domain.PendingSelection{}, err
	}
	if !found {
		return domain.PendingSelection{}, ErrRecommendationNotFound
	}

	sel, err := s.buildSelection(ctx, vpa, cr, opts, true)
	if err != nil {
		return domain.PendingSelection{}, err
	}
	if err := s.persistSelections(ctx, []domain.PendingSelection{sel}); err != nil {
		return domain.PendingSelection{}, err
	}
	return sel, nil
}

// BulkSelectRecommendations queues the requested resource(s) for every
// opted-in, eligible container in one batch, skipping (not failing on)
// containers where the requested resource(s) aren't configured or
// individually eligible. All selections are written in a single
// StateStore.Update rather than one per container.
func (s *Service) BulkSelectRecommendations(ctx context.Context, opts SelectOptions) (BulkSelectResponse, error) {
	if err := opts.validate(); err != nil {
		return BulkSelectResponse{}, err
	}

	vpas, err := s.Reader.ListOptedIn(ctx)
	if err != nil {
		return BulkSelectResponse{}, fmt.Errorf("api: list opted-in VPAs: %w", err)
	}

	var result BulkSelectResponse
	for _, vpa := range vpas {
		if !vpa.Valid {
			continue
		}
		for _, cc := range vpa.Binding.Containers {
			cr, found := vpa.ContainerByName(cc.ContainerName)
			if !found {
				result.Skipped = append(result.Skipped, SkippedSelectionDTO{
					Namespace: vpa.Namespace, VPAName: vpa.Name, ContainerName: cc.ContainerName,
					Reason: "no recommendation yet",
				})
				continue
			}
			sel, err := s.buildSelection(ctx, vpa, cr, opts, false)
			if err != nil {
				result.Skipped = append(result.Skipped, SkippedSelectionDTO{
					Namespace: vpa.Namespace, VPAName: vpa.Name, ContainerName: cc.ContainerName,
					Reason: err.Error(),
				})
				continue
			}
			result.Selected = append(result.Selected, sel)
		}
	}

	if err := s.persistSelections(ctx, result.Selected); err != nil {
		return BulkSelectResponse{}, err
	}
	return result, nil
}

func toWorkloadDTO(w domain.WorkloadRef) WorkloadDTO {
	return WorkloadDTO{Kind: w.Kind, Name: w.Name, Namespace: w.Namespace}
}

func quantityString(q *resource.Quantity) string {
	if q == nil {
		return ""
	}
	return q.String()
}

func finitePercent(d eligibility.Delta) *float64 {
	if !d.HasCurrent || !d.HasRecommended {
		return nil
	}
	v := d.PercentChange
	if v != v || v > 1e18 || v < -1e18 { // NaN or effectively infinite
		return nil
	}
	return &v
}

// operationFor returns the most recently updated Operations entry for
// (namespace, vpaName, containerName), or nil if there is none.
func operationFor(doc *domain.StateDocument, namespace, vpaName, containerName string) *domain.OperationState {
	var latest *domain.OperationState
	for k := range doc.Operations {
		op := doc.Operations[k]
		if op.VPANamespace != namespace || op.VPAName != vpaName || op.ContainerName != containerName {
			continue
		}
		if latest == nil || op.UpdatedAt.After(latest.UpdatedAt) {
			opCopy := op
			latest = &opCopy
		}
	}
	return latest
}

// statusFor looks up the most relevant known state for (namespace, vpaName,
// containerName): a pending selection wins over a past operation, and among
// operations the most recently updated one wins. Absent any entry, "new" is
// the default per the acceptance criteria's status enum.
func statusFor(doc *domain.StateDocument, namespace, vpaName, containerName string) string {
	for _, sel := range doc.PendingSelections {
		if sel.VPANamespace == namespace && sel.VPAName == vpaName && sel.ContainerName == containerName {
			return "selected"
		}
	}

	latest := operationFor(doc, namespace, vpaName, containerName)
	if latest == nil {
		return "new"
	}

	switch latest.Status {
	case domain.OperationApplying:
		return "applying"
	case domain.OperationApplied:
		return "applied"
	case domain.OperationFailed, domain.OperationConflict:
		return "failed"
	default:
		return "new"
	}
}
