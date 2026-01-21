{{/*
Expand the name of the chart.
*/}}
{{- define "maia.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "maia.fullname" -}}
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

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "maia.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "maia.labels" -}}
helm.sh/chart: {{ include "maia.chart" . }}
{{ include "maia.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "maia.selectorLabels" -}}
app.kubernetes.io/name: {{ include "maia.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "maia.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "maia.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Get the API key secret name
*/}}
{{- define "maia.apiKeySecretName" -}}
{{- if .Values.maia.security.existingSecret }}
{{- .Values.maia.security.existingSecret }}
{{- else }}
{{- include "maia.fullname" . }}-api-key
{{- end }}
{{- end }}

{{/*
Get the OpenAI API key secret name
*/}}
{{- define "maia.openaiSecretName" -}}
{{- if .Values.maia.embedding.openai.existingSecret }}
{{- .Values.maia.embedding.openai.existingSecret }}
{{- else }}
{{- include "maia.fullname" . }}-openai
{{- end }}
{{- end }}

{{/*
Return true if we should create an API key secret
*/}}
{{- define "maia.createApiKeySecret" -}}
{{- if and .Values.maia.security.apiKey (not .Values.maia.security.existingSecret) }}
{{- true }}
{{- end }}
{{- end }}

{{/*
Return true if we should create an OpenAI secret
*/}}
{{- define "maia.createOpenaiSecret" -}}
{{- if and .Values.maia.embedding.openai.apiKey (not .Values.maia.embedding.openai.existingSecret) }}
{{- true }}
{{- end }}
{{- end }}
