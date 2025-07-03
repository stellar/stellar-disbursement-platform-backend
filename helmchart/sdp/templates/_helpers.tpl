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
{{- .Values.sdp.route.domain | default (printf "localhost:%s" (include "sdp.port" .)) }}
{{- end }}

{{/*
SDP MTN domain
*/}}
{{- define "sdp.mtnDomain" -}}
{{- .Values.sdp.route.mtnDomain | default (printf "localhost:%s" (include "sdp.port" .)) }}
{{- end }}

{{/*
SDP ADMIN domain
*/}}
{{- define "sdp.adminDomain" -}}
{{- .Values.sdp.route.adminDomain | default "" }}
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
{{- .Values.anchorPlatform.route.domain | default (printf "localhost:%s" (include "sdp.ap.sepPort" .)) }}
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
{{- define "sdp.dashboard.domain" -}}
{{- .Values.dashboard.route.domain | default (printf "localhost:%s" (include "sdp.dashboard.port" .)) }}
{{- end }}

{{/*
Dashboard MTN domain
*/}}
{{- define "sdp.dashboard.mtnDomain" -}}
{{- .Values.dashboard.route.mtnDomain | default (printf "localhost:%s" (include "sdp.dashboard.port" .)) }}
{{- end }}

{{/*
Dashboard domain schema
*/}}
{{- define "sdp.dashboard.schema" -}}
{{- .Values.dashboard.route.schema | default "https" }}
{{- end }}

{{/*
Dashboard port
*/}}
{{- define "sdp.dashboard.port" -}}
{{- .Values.dashboard.route.port | default "80" }}
{{- end }}


{{/*
Is Pubnet?
*/}}
{{- define "isPubnet" -}}
{{- eq .Values.global.isPubnet true | default false }}
{{- end }}


{{/*
USDC Issuer depending on network
*/}}
{{- define "sdp.usdcIssuer" -}}
{{- if eq .Values.global.isPubnet true -}}
GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN
{{- else -}}
GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5
{{- end -}}
{{- end -}}

{{/*
EURC Issuer depending on network
*/}}
{{- define "sdp.eurcIssuer" -}}
{{- if eq .Values.global.isPubnet true -}}
GDHU6WRG4IEQXM5NZ4BMPKOXHW76MZM4Y2IEMFDVXBSDP6SJY4ITNPP2
{{- else -}}
GB3Q6QDZYTHWT7E5PVS3W7FUT5GVAFC5KSZFFLPU25GO7VTC3NM2ZTVO
{{- end -}}
{{- end -}}

{{/*
Generate JWT secret - generates once per template rendering
Returns: A 32-character random alphanumeric string
*/}}
{{- define "sdp.jwtSecret" -}}
{{- $jwtSecret := default (randAlphaNum 32) .Values._generatedJwtSecret -}}
{{- $_ := set .Values "_generatedJwtSecret" $jwtSecret -}}
{{- $jwtSecret -}}
{{- end -}}

{{/*
Generate platform auth secret - generates once per template rendering
Returns: A 32-character random alphanumeric string
*/}}
{{- define "sdp.platformAuthSecret" -}}
{{- $authSecret := default (randAlphaNum 32) .Values._generatedPlatformAuthSecret -}}
{{- $_ := set .Values "_generatedPlatformAuthSecret" $authSecret -}}
{{- $authSecret -}}
{{- end -}}

{{/*
Generate admin account name - generates once per template rendering
Returns: Default "sdp-admin" or user-provided value
*/}}
{{- define "sdp.adminAccount" -}}
{{- $adminAccount := default "sdp-admin" .Values._generatedAdminAccount -}}
{{- $_ := set .Values "_generatedAdminAccount" $adminAccount -}}
{{- $adminAccount -}}
{{- end -}}

{{/*
Generate admin API key - generates once per template rendering
Returns: A 32-character random alphanumeric string
*/}}
{{- define "sdp.adminApiKey" -}}
{{- $adminApiKey := default (randAlphaNum 32) .Values._generatedAdminApiKey -}}
{{- $_ := set .Values "_generatedAdminApiKey" $adminApiKey -}}
{{- $adminApiKey -}}
{{- end -}}

{{/*
Generate EC256 private key - generates once per template rendering
Returns: ECDSA private key in PEM format
*/}}
{{- define "sdp.ec256PrivateKey" -}}
{{- $key := default (genPrivateKey "ecdsa") .Values._generatedEC256Key -}}
{{- $_ := set .Values "_generatedEC256Key" $key -}}
{{- $key -}}
{{- end -}}

{{/*
SDP base URL with schema and domain
*/}}
{{- define "sdp.baseURL" -}}
{{- printf "%s://%s" (include "sdp.schema" .) (include "sdp.domain" .) -}}
{{- end -}}

{{/*
AP SEP base URL with schema and domain
*/}}
{{- define "sdp.ap.baseURL" -}}
{{- printf "%s://%s" (include "sdp.ap.schema" .) (include "sdp.ap.domain" .) -}}
{{- end -}}

{{/*
Dashboard base URL with schema and domain
*/}}
{{- define "sdp.dashboard.baseURL" -}}
{{- printf "%s://%s" (include "sdp.dashboard.schema" .) (include "sdp.dashboard.domain" .) -}}
{{- end -}}

{{/*
Bridge API base URL depending on network
*/}}
{{- define "sdp.bridge.baseURL" -}}
{{- if eq .Values.global.isPubnet true -}}
https://api.bridge.xyz
{{- else -}}
https://api.sandbox.bridge.xyz
{{- end -}}
{{- end -}}

