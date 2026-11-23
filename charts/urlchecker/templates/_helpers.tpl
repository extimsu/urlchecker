{{/* Chart name, truncated for DNS-1123 labels. */}}
{{- define "urlchecker.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/* Fully qualified name: release, or release-chart. */}}
{{- define "urlchecker.fullname" -}}
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

{{- define "urlchecker.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "urlchecker.selectorLabels" -}}
app.kubernetes.io/name: {{ include "urlchecker.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "urlchecker.labels" -}}
helm.sh/chart: {{ include "urlchecker.chart" . }}
{{ include "urlchecker.selectorLabels" . }}
app.kubernetes.io/version: {{ .Values.image.tag | default .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{- define "urlchecker.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "urlchecker.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/* registry/repository:tag, or @digest when a digest is set. */}}
{{- define "urlchecker.image" -}}
{{- $repo := printf "%s/%s" .Values.image.registry .Values.image.repository | trimPrefix "/" }}
{{- if .Values.image.digest }}
{{- printf "%s@%s" $repo .Values.image.digest }}
{{- else }}
{{- printf "%s:%s" $repo (.Values.image.tag | default .Chart.AppVersion) }}
{{- end }}
{{- end }}

{{- define "urlchecker.configMapName" -}}
{{- default (include "urlchecker.fullname" .) .Values.existingConfigMap }}
{{- end }}

{{/* Fail early on combinations that cannot work. */}}
{{- define "urlchecker.validate" -}}
{{- if not (has .Values.workload.kind (list "Deployment" "DaemonSet")) }}
{{- fail "workload.kind must be Deployment or DaemonSet" }}
{{- end }}
{{- if and (not .Values.existingConfigMap) (not .Values.targets) }}
{{- fail "set targets (or existingConfigMap): urlchecker needs at least one target" }}
{{- end }}
{{- if and .Values.serviceMonitor.enabled (not .Values.service.enabled) }}
{{- fail "serviceMonitor.enabled needs service.enabled" }}
{{- end }}
{{- if and .Values.hostNetwork (not .Values.icmp.rawSocket) }}
{{- range .Values.targets }}
{{- if hasPrefix "icmp://" (lower .) }}
{{- fail "icmp:// targets with hostNetwork need icmp.rawSocket=true: pod sysctls cannot change the node's ping_group_range" }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}
