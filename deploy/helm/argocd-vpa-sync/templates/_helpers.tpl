{{- define "argocd-vpa-sync.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "argocd-vpa-sync.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{- define "argocd-vpa-sync.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{ include "argocd-vpa-sync.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "argocd-vpa-sync.selectorLabels" -}}
app.kubernetes.io/name: {{ include "argocd-vpa-sync.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "argocd-vpa-sync.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "argocd-vpa-sync.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "argocd-vpa-sync.stateSecretName" -}}
{{- default (printf "%s-state" (include "argocd-vpa-sync.fullname" .)) .Values.state.secretName }}
{{- end }}

{{/* Secret holding admin.password and server.secretkey. */}}
{{- define "argocd-vpa-sync.authSecretName" -}}
{{- default (printf "%s-secret" (include "argocd-vpa-sync.fullname" .)) .Values.auth.existingSecret }}
{{- end }}

{{- define "argocd-vpa-sync.initialAdminSecretName" -}}
{{- printf "%s-initial-admin-secret" (include "argocd-vpa-sync.fullname" .) }}
{{- end }}

{{- define "argocd-vpa-sync.knownHostsConfigMap" -}}
{{- if .Values.ssh.extraKnownHosts }}
{{- printf "%s-ssh-known-hosts" (include "argocd-vpa-sync.fullname" .) }}
{{- else }}
{{- .Values.ssh.knownHostsConfigMap }}
{{- end }}
{{- end }}
