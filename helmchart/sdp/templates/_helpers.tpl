{{/*
Expand the name of the chart.
*/}}
{{- define "sdp.name" -}}
{{- default .Chart.Name .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "sdp.fullname" -}}
{{- default .Chart.Name .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "sdp.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}


{{/*
Common labels with suffix
*/}}
{{- define "sdp.labelsWithSuffix" -}}
{{- $ctx := index . 0 -}}
{{- $suffix := index . 1 | default "" -}}
helm.sh/chart: {{ include "sdp.chart" $ctx }}
{{ include "sdp.selectorLabelsWithSuffix" (list $ctx $suffix) }}
{{- if $ctx.Chart.AppVersion }}
app.kubernetes.io/version: {{ $ctx.Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ $ctx.Release.Service }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "sdp.labels" -}}
helm.sh/chart: {{ include "sdp.chart" . }}
{{ include "sdp.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}


{{/*
Selector labels with suffix
*/}}
{{- define "sdp.selectorLabelsWithSuffix" -}}
{{- $ctx := index . 0 -}}
{{- $suffix := index . 1 | default "" -}}
app.kubernetes.io/name: {{ include "sdp.name" $ctx }}{{ $suffix }}
app.kubernetes.io/instance: {{ $ctx.Release.Name }}{{ $suffix }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "sdp.selectorLabels" -}}
{{ include "sdp.selectorLabelsWithSuffix" (list . "") }}
{{- end }}


{{/*
SDP domain
*/}}
{{- define "sdp.domain" -}}
{{- .Values.sdp.route.domain | default "localhost" }}
{{- end }}

{{/*
SDP MTN domain
*/}}
{{- define "sdp.mtnDomain" -}}
{{- .Values.sdp.route.mtnDomain | default "localhost" }}
{{- end }}

{{/*
SDP domain schema
*/}}
{{- define "sdp.schema" -}}
{{- .Values.sdp.route.schema | default "https" }}
{{- end }}

{{/*
SDP port
*/}}
{{- define "sdp.port" -}}
{{- .Values.sdp.route.port | default "8000" }}
{{- end }}

{{/*
SDP Metrics port
*/}}
{{- define "sdp.metricsPort" -}}
{{- .Values.sdp.route.metricsPort | default "8002" }}
{{- end }}

{{/*
SDP Admin port
*/}}
{{- define "sdp.adminPort" -}}
{{- .Values.sdp.route.adminPort | default "8003" }}
{{- end }}

{{/*
Define the full address to the SDP service.
*/}}
{{- define "sdp.serviceAddress" -}}
http://{{ include "sdp.fullname" . }}.{{ .Release.Namespace }}.svc.cluster.local:{{ include "sdp.port" . }}
{{- end -}}


{{/*
TSS port
*/}}
{{- define "tss.port" -}}
{{- .Values.tss.route.port | default "9000" }}
{{- end }}

{{/*
TSS Metrics port
*/}}
{{- define "tss.metricsPort" -}}
{{- .Values.tss.route.metricsPort | default "9002" }}
{{- end }}


{{/*
Anchor Platform domain
*/}}
{{- define "sdp.ap.domain" -}}
{{- .Values.anchorPlatform.route.domain | default "localhost" }}
{{- end }}

{{/*
Anchor Platform schema
*/}}
{{- define "sdp.ap.schema" -}}
{{- .Values.anchorPlatform.route.schema | default "https" }}
{{- end }}

{{/*
Anchor Platform SEP/public port
*/}}
{{- define "sdp.ap.sepPort" -}}
{{- .Values.anchorPlatform.route.sepPort | default "8080" }}
{{- end }}

{{/*
Anchor Platform internal communication port
*/}}
{{- define "sdp.ap.platformPort" -}}
{{- .Values.anchorPlatform.route.platformPort | default "8085" }}
{{- end }}

{{/*
Anchor Platform metrics port
*/}}
{{- define "sdp.ap.metricsPort" -}}
{{- 8082 }}
{{- end }}

{{/*
AP SEP full service address
*/}}
{{- define "sdp.ap.sepServiceAddress" -}}
http://{{ include "sdp.fullname" . }}-ap.{{ .Release.Namespace }}.svc.cluster.local:{{ include "sdp.ap.sepPort" . }}
{{- end -}}

{{/*
AP Platform full service address
*/}}
{{- define "sdp.ap.platformServiceAddress" -}}
http://{{ include "sdp.fullname" . }}-ap.{{ .Release.Namespace }}.svc.cluster.local:{{ include "sdp.ap.platformPort" . }}
{{- end -}}


{{/*
Dashboard domain
*/}}
{{- define "dashboard.domain" -}}
{{- .Values.dashboard.route.domain | default "localhost" }}
{{- end }}

{{/*
Dashboard MTN domain
*/}}
{{- define "dashboard.mtnDomain" -}}
{{- .Values.dashboard.route.mtnDomain | default "localhost" }}
{{- end }}

{{/*
Dashboard domain schema
*/}}
{{- define "dashboard.schema" -}}
{{- .Values.dashboard.route.schema | default "https" }}
{{- end }}

{{/*
Dashboard port
*/}}
{{- define "dashboard.port" -}}
{{- .Values.dashboard.route.port | default "80" }}
{{- end }}


{{/*
Is Pubnet?
*/}}
{{- define "isPubnet" -}}
{{- eq .Values.global.isPubnet true | default false }}
{{- end }}

{{/*
Image Tag
*/}}
{{- define "imageTag" -}}
{{- .Values.sdp.image.tag | default .Chart.AppVersion }}
{{- end }}
