package main

const resetSourceTemplate = `{{.Header}}package {{.PackageName}}

{{range .Types}}// Reset restores the fields of {{.Name}} to their initial state.
func (r *{{.ReceiverType}}) Reset() {
	if r == nil {
		return
	}
{{range .Statements}}{{.}}
{{end}}}

{{end}}func {{.ZeroHelperName}}[T any](_ T) T {
	var zero T
	return zero
}
`

type packageTemplateData struct {
	Header         string
	PackageName    string
	Types          []resetTemplateData
	ZeroHelperName string
}

type resetTemplateData struct {
	Name         string
	ReceiverType string
	Statements   []string
}
