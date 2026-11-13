{{/* The chart name. */}}
{{- define "dendronotes.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
The name of the objects: "dendronotes" by default (also when the release has
another name), so PVCs and the Secret keep their documented names.
*/}}
{{- define "dendronotes.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- include "dendronotes.name" . }}
{{- end }}
{{- end }}

{{- define "dendronotes.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{ include "dendronotes.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "dendronotes.selectorLabels" -}}
app.kubernetes.io/name: {{ include "dendronotes.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: server
{{- end }}

{{- define "dendronotes.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "dendronotes.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "dendronotes.secretName" -}}
{{- default (printf "%s-auth" (include "dendronotes.fullname" .)) .Values.auth.existingSecret }}
{{- end }}

{{/* The Secret key with the admin credential ("" for none). */}}
{{- define "dendronotes.adminKey" -}}
{{- if .Values.auth.existingSecret }}
{{- .Values.auth.admin.secretKey }}
{{- else if .Values.auth.admin.passwordHash }}
{{- "admin-password-hash" }}
{{- else }}
{{- "admin-password" }}
{{- end }}
{{- end }}

{{- define "dendronotes.oidcKey" -}}
{{- if .Values.auth.existingSecret }}{{ .Values.auth.oidcClientSecretKey }}{{ else if .Values.auth.oidcClientSecret }}oidc-client-secret{{ end }}
{{- end }}

{{- define "dendronotes.smtpKey" -}}
{{- if .Values.auth.existingSecret }}{{ .Values.auth.smtpPasswordKey }}{{ else if .Values.auth.smtpPassword }}smtp-password{{ end }}
{{- end }}

{{- define "dendronotes.ldapKey" -}}
{{- if .Values.auth.existingSecret }}{{ .Values.auth.ldapBindPasswordKey }}{{ else if .Values.auth.ldapBindPassword }}ldap-bind-password{{ end }}
{{- end }}

{{- define "dendronotes.claimName" -}}
{{- $root := index . 0 }}{{ $which := index . 1 }}
{{- $p := index $root.Values.persistence $which }}
{{- default (printf "%s-%s" (include "dendronotes.fullname" $root) $which) $p.existingClaim }}
{{- end }}
