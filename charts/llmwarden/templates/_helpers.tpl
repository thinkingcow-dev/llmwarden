{{/*
Expand the name of the chart.
*/}}
{{- define "llmwarden.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "llmwarden.fullname" -}}
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
{{- define "llmwarden.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "llmwarden.labels" -}}
helm.sh/chart: {{ include "llmwarden.chart" . }}
{{ include "llmwarden.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "llmwarden.selectorLabels" -}}
app.kubernetes.io/name: {{ include "llmwarden.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
control-plane: controller-manager
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "llmwarden.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "llmwarden.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the webhook service
*/}}
{{- define "llmwarden.webhookServiceName" -}}
{{- printf "%s-webhook" (include "llmwarden.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create the name of the metrics service
*/}}
{{- define "llmwarden.metricsServiceName" -}}
{{- printf "%s-metrics" (include "llmwarden.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create the name of the certificate
*/}}
{{- define "llmwarden.certificateName" -}}
{{- printf "%s-serving-cert" (include "llmwarden.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create the name of the webhook secret
*/}}
{{- define "llmwarden.webhookSecretName" -}}
{{- printf "%s-webhook-server-cert" (include "llmwarden.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common annotations
*/}}
{{- define "llmwarden.annotations" -}}
{{- with .Values.commonAnnotations }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Image name
*/}}
{{- define "llmwarden.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion }}
{{- printf "%s:%s" .Values.image.repository $tag }}
{{- end }}

{{/*
Leader election resource namespace
*/}}
{{- define "llmwarden.leaderElectionNamespace" -}}
{{- default .Release.Namespace .Values.leaderElection.resourceNamespace }}
{{- end }}

{{/*
ServiceMonitor namespace
*/}}
{{- define "llmwarden.serviceMonitorNamespace" -}}
{{- default .Release.Namespace .Values.metrics.serviceMonitor.namespace }}
{{- end }}
