{{- define "tamga-proxy.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "tamga-proxy.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "tamga-proxy.labels" -}}
app.kubernetes.io/name: {{ include "tamga-proxy.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end -}}

{{- define "tamga-proxy.selectorLabels" -}}
app.kubernetes.io/name: {{ include "tamga-proxy.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "tamga-proxy.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "tamga-proxy.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "tamga-proxy.secretName" -}}
{{- default (include "tamga-proxy.fullname" .) .Values.secrets.existingSecret -}}
{{- end -}}

{{/* The proxy serves HTTPS when both TLS variables are set, and the probes must follow. */}}
{{- define "tamga-proxy.scheme" -}}
{{- $env := .Values.env | default dict -}}
{{- if and (get $env "TAMGA_TLS_CERT") (get $env "TAMGA_TLS_KEY") -}}HTTPS{{- else -}}HTTP{{- end -}}
{{- end -}}
