{{- define "argocd-vpa-updater.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "argocd-vpa-updater.fullname" -}}
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

{{- define "argocd-vpa-updater.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{ include "argocd-vpa-updater.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "argocd-vpa-updater.selectorLabels" -}}
app.kubernetes.io/name: {{ include "argocd-vpa-updater.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "argocd-vpa-updater.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "argocd-vpa-updater.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "argocd-vpa-updater.stateSecretName" -}}
{{- default (printf "%s-state" (include "argocd-vpa-updater.fullname" .)) .Values.state.secretName }}
{{- end }}

{{/* Secret holding admin.password and server.secretkey. */}}
{{- define "argocd-vpa-updater.authSecretName" -}}
{{- default (printf "%s-secret" (include "argocd-vpa-updater.fullname" .)) .Values.auth.existingSecret }}
{{- end }}

{{- define "argocd-vpa-updater.initialAdminSecretName" -}}
{{- printf "%s-initial-admin-secret" (include "argocd-vpa-updater.fullname" .) }}
{{- end }}

{{- define "argocd-vpa-updater.knownHostsConfigMap" -}}
{{- if .Values.ssh.extraKnownHosts }}
{{- printf "%s-ssh-known-hosts" (include "argocd-vpa-updater.fullname" .) }}
{{- else }}
{{- .Values.ssh.knownHostsConfigMap }}
{{- end }}
{{- end }}
