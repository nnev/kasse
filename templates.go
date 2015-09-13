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
		// Skip hidden files
		if filepath.Base(f)[0] == '.' {
			continue
		}
		content, err := ioutil.ReadFile(f)
		if err != nil {
			log.Fatalf("Could not read %q: %v", f, err)
		}

		t := template.New("page")
		t.Funcs(map[string]interface{}{
			"toEuros": func(x int) float64 {
				return float64(x) / 100
			},
		})

		t = template.Must(t.Parse(string(layout)))
		template.Must(t.New("content").Parse(string(content)))

		parsedTemplates[filepath.Base(f)] = t
	}
}

// ExecuteTemplate executes a template to w.
func ExecuteTemplate(w io.Writer, data TemplateInput) error {
	return parsedTemplates[data.Body].Execute(w, data)
}
