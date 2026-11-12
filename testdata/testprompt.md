{{- if .HasTool "search" }}
you have access to the search tool. use it to find real-time information.
{{ else -}}
you do not have search capabilities.
{{ end -}}

current time: {{ .CurrentTime }}
working directory: {{ .WorkingDirectory }}
extra info: {{ .Extra }}

{{ range .Tools }}
- tool: {{ . }}
{{ end }}
