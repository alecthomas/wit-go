package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/alecthomas/kong"
	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/repr"
)

type AST struct {
	Decls []*Decl `@@*`
}

type Decl struct {
	Func *Func `@@`
}

// eg. send-receive-skip-search : func(process-id: u64, timeout: u32) -> u32
type Func struct {
	Name string `@(Ident ("-" Ident)*)`
	Args []*Arg `":" "func" "(" (@@ ("," @@)* ","?)? ")"`
	Ret  *Ret   `("-" ">" @@)?`
}

type Arg struct {
	Name string `@(Ident ("-" Ident)*)`
	Type *Type  `":" @@`
}

type Ret struct {
	Type *Type `@@`
}

type Type struct {
	Ident string `@Ident`
}

var (
	parser = participle.MustBuild[AST]()

	witTemplate = template.Must(
		template.New("wit").
			Funcs(template.FuncMap{
				"Public": func(s string) string {
					parts := strings.Split(s, "-")
					for i := range parts {
						parts[i] = strings.Title(parts[i])
					}
					return strings.Join(parts, "")
				},
				"Private": func(s string) string {
					parts := strings.Split(s, "-")
					for i := range parts {
						if i > 0 {
							parts[i] = strings.Title(parts[i])
						} else {
							parts[i] = strings.ToLower(parts[i])
						}
					}
					return strings.Join(parts, "")
				},
				"Type": func(t *Type) string {
					switch t.Ident {
					case "u8":
						return "uint8"
					case "u16":
						return "uint16"
					case "u32":
						return "uint32"
					case "u64":
						return "uint64"
					case "s8":
						return "int8"
					case "s16":
						return "int16"
					case "s32":
						return "int32"
					case "s64":
						return "int64"
					case "float32":
						return "float32"
					case "float64":
						return "float64"
					default:
						panic("unknown type")
					}
				},
			}).
			Parse(`package {{ .Package }}

{{ range .AST.Decls -}}
{{with .Func -}}
//go:wasm-module {{$.Module}}
//go:export {{.Name|Public}}
func {{.Name|Public}}(
{{- range $idx, $arg := .Args -}}
{{- if $idx}}, {{end -}}
{{- .Name|Private}} {{.Type|Type}}
{{- end -}}
){{- if .Ret}} {{.Ret.Type|Type}}{{end}}

{{end -}}
{{end}}
`))
	cli struct {
		Dump  bool     `required:"" xor:"action" help:"Dump the AST."`
		Dest  string   `short:"o" required:"" xor:"action" help:"Destination directory."`
		Files []string `required:"" arg:"" type:"existingfile" help:"Files to generate from."`
	}
)

func main() {
	kctx := kong.Parse(&cli)

	for _, file := range cli.Files {
		f, err := os.Open(file)
		kctx.FatalIfErrorf(err)
		defer f.Close()

		ast, err := parser.Parse(file, f)
		kctx.FatalIfErrorf(err)

		if cli.Dump {
			repr.Println(ast)
		} else {
			kctx.FatalIfErrorf(codegen(cli.Dest, file, ast))
		}
	}
}

func codegen(dest, filename string, ast *AST) error {
	name := strings.TrimSuffix(filepath.Base(filename), ".wit")
	// wit/lunatic_timer.wit -> lunatic::timer
	module := strings.ReplaceAll(name, "_", "::")
	pkgParts := strings.Split(name, "_")
	path := filepath.Join(pkgParts...)
	pkg := pkgParts[len(pkgParts)-1]

	err := os.MkdirAll(filepath.Join(dest, path), 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	w, err := os.Create(filepath.Join(dest, path, pkg+".go"))
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer w.Close()

	return witTemplate.Execute(w, struct {
		Module  string
		Package string
		AST     *AST
	}{
		Module:  module,
		Package: pkg,
		AST:     ast,
	})
}
