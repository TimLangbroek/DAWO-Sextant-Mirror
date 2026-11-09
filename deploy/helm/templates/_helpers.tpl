{{/*
sextant.gib renders a Kubernetes quantity as a number of GiB, so a template can
compare a size against a floor. Helm has no unit parser, and a guard that only
understood "8Gi" would pass silently on "500Mi" - which is the case it exists
to catch.

Binary and decimal suffixes both, because a values file written by hand carries
whichever the author had in mind, and treating 1G as 1Gi would only ever err
towards allowing a volume that is too small.
*/}}
{{- define "sextant.gib" -}}
{{- $q := . | toString -}}
{{- $n := regexFind "^[0-9]+(\\.[0-9]+)?" $q | float64 -}}
{{- $u := regexFind "[A-Za-z]*$" $q -}}
{{- if eq $u "Ti" -}}{{ mulf $n 1024.0 }}
{{- else if eq $u "Gi" -}}{{ $n }}
{{- else if eq $u "Mi" -}}{{ divf $n 1024.0 }}
{{- else if eq $u "Ki" -}}{{ divf $n 1048576.0 }}
{{- else if eq $u "T" -}}{{ divf (mulf $n 1000000000000.0) 1073741824.0 }}
{{- else if eq $u "G" -}}{{ divf (mulf $n 1000000000.0) 1073741824.0 }}
{{- else if eq $u "M" -}}{{ divf (mulf $n 1000000.0) 1073741824.0 }}
{{- else -}}{{ divf $n 1073741824.0 }}
{{- end -}}
{{- end -}}

{{/*
sextant.gateRunner says whether the release runs a gate-runner: always with
gateMode remote, which cannot work without one, and on request before that,
so a runner can be warmed up ahead of switching the console to it (ADR 0012).
*/}}
{{- define "sextant.gateRunner" -}}
{{- if or .Values.gateRunner.enabled (eq .Values.gateMode "remote") -}}true{{- end -}}
{{- end -}}

{{/*
sextant.gateTokenRef is where the gate-runner's bearer token comes from: the
app secret when it carries GATE_TOKEN (set by hand, as before), otherwise the
Secret this chart generates once and keeps (gate-runner.yaml). lookup sees
nothing under `helm template`, so a plain render always shows the generated
one.
*/}}
{{- define "sextant.gateTokenOwn" -}}
{{- $app := lookup "v1" "Secret" .Release.Namespace .Values.secretName -}}
{{- if not (and $app (hasKey ($app.data | default dict) "GATE_TOKEN")) -}}true{{- end -}}
{{- end -}}
{{- define "sextant.gateTokenRef" -}}
{{- if include "sextant.gateTokenOwn" . -}}
name: {{ .Release.Name }}-gate-token
key: token
{{- else -}}
name: {{ .Values.secretName }}
key: GATE_TOKEN
{{- end -}}
{{- end -}}

{{/*
sextant.dbMode resolves database.mode with the pre-0.93 behaviour as the
fallback, so values files that only know cnpg.enabled keep rendering the same.
*/}}
{{- define "sextant.dbMode" -}}
{{- if .Values.database.mode -}}{{ .Values.database.mode }}
{{- else if .Values.cnpg.enabled -}}cnpg
{{- else -}}none
{{- end -}}
{{- end -}}

{{/*
sextant.routeKind resolves route.kind the same way from ingress.enabled.
*/}}
{{- define "sextant.routeKind" -}}
{{- if .Values.route.kind -}}{{ .Values.route.kind }}
{{- else if .Values.ingress.enabled -}}ingress
{{- else -}}none
{{- end -}}
{{- end -}}

{{/*
sextant.pgSelector is the label that selects the database pods, for the
network policies: CNPG labels its instances, the embedded StatefulSet is ours.
*/}}
{{- define "sextant.pgSelector" -}}
{{- if eq (include "sextant.dbMode" .) "cnpg" -}}
cnpg.io/cluster: {{ .Values.cnpg.name }}
{{- else -}}
app: {{ .Release.Name }}-pg
{{- end -}}
{{- end -}}
