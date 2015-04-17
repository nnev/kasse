package main

import (
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"path/filepath"
)

// TemplateInput is the input to a rendered Template. Body should name a
// template-file. Data will be provided to the Body-Template.
type TemplateInput struct {
	Title string
	Body  string
	Data  interface{}
}

var (
	parsedTemplates = make(map[string]*template.Template)
)

func init() {
	layout, err := ioutil.ReadFile("templates/layout.html")
	if err != nil {
		log.Fatal("Could not read layout:", err)
	}
	files, err := filepath.Glob("templates/*")
	if err != nil {
		log.Fatal("Could not glob templates:", err)
	}

	for _, f := range files {
		if filepath.Base(f) == "layout.html" {
			continue
		}
		content, err := ioutil.ReadFile(f)
		if err != nil {
			log.Fatalf("Could not read %q: %v", f, err)
		}
		t := template.Must(template.New("page").Parse(string(layout)))
		template.Must(t.New("content").Parse(string(content)))
		parsedTemplates[filepath.Base(f)] = t
	}
}

// ExecuteTemplate executes a template to w.
func ExecuteTemplate(w io.Writer, data TemplateInput) error {
	return parsedTemplates[data.Body].Execute(w, data)
}
