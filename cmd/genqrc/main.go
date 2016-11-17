
// XXX: The documentation is duplicated here and in the the doc variable
// below. Update both at the same time.

// Command genqrc packs resource files into the Go binary.
//
// Usage: genqrc [options] <path1> [<path2> ...]
//
// The genqrc tool packs all resource files under the provided paths into
// a single qrc.go file that may be built into the generated binary. Bundled files
// may then be loaded by Go or QML code under the URL "qrc:///some/path", where
// "some/path" matches the original path for the resource file locally.
//
// paths can be:
//   * a .qrc filename, as defined by http://doc.qt.io/qt-5/resources.html and built by
//       Qt Creator. This is the preferred method
//   * a filename. The file will be imported directly
//   * a directory. all files within the directory will be imported
//
// For example, the following will load a .qml file from the resource pack, and
// that file may in turn reference other content (code, images, etc) in the pack:
//
//     component, err := engine.LoadFile("qrc://path/to/file.qml")
//
// Starting with Go 1.4, this tool may be conveniently run by the "go generate"
// subcommand by adding a line similar to the following one to any existent .go
// file in the project (assuming the subdirectories ./code/ and ./images/ exist):
//
//     //go:generate genqrc qml.qrc main.qml code images
//
// Then, just run "go generate" to update the qrc.go file.
//
// During development, the generated qrc.go can repack the filesystem content at
// runtime to avoid the process of regenerating the qrc.go file and rebuilding the
// application to test every minor change made. Runtime repacking is enabled by
// setting the QRC_REPACK environment variable to 1:
//
//     export QRC_REPACK=1
//
// This does not update the static content in the qrc.go file, though, so after
// the changes are performed, genqrc must be run again to update the content that
// will ship with built binaries.
//
// NOTES:
//     * Files labeled *.qrc are not parsed unless explicitely set in the parameters list.
//     * All *.qmltypes files are ignored, since they are not intended to be useful to the compiled code.
//     * qmldir files are currently ignored and so import definitions need to be handled accordingly. Would love for this to change.

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"text/template"
	"encoding/xml"

	"gopkg.in/qml.v1"
)

// XXX: The documentation is duplicated here and in the the package comment
// above. Update both at the same time.

const doc = `
** Modified **
Usage: genqrc [options] <path1> [<path2> ...]

The genqrc tool packs all resource files under the provided paths into
a single qrc.go file that may be built into the generated binary. Bundled files
may then be loaded by Go or QML code under the URL "qrc:///some/path", where
"some/path" matches the original path for the resource file locally.

paths can be:
  * a *.qrc filename, as defined by http://doc.qt.io/qt-5/resources.html and built by
      Qt Creator. This is the preferred method
  * a filename. The file will be imported directly
  * a directory. all files within the directory will be imported

For example, the following will load a .qml file from the resource pack, and
that file may in turn reference other content (code, images, etc) in the pack:

    component, err := engine.LoadFile("qrc://path/to/file.qml")

Starting with Go 1.4, this tool may be conveniently run by the "go generate"
subcommand by adding a line similar to the following one to any existent .go
file in the project (assuming the subdirectories ./code/ and ./images/ exist):

    //go:generate genqrc qml.qrc main.qml code images

Then, just run "go generate" to update the qrc.go file.

During development, the generated qrc.go can repack the filesystem content at
runtime to avoid the process of regenerating the qrc.go file and rebuilding the
application to test every minor change made. Runtime repacking is enabled by
setting the QRC_REPACK environment variable to 1:

    export QRC_REPACK=1

This does not update the static content in the qrc.go file, though, so after
the changes are performed, genqrc must be run again to update the content that
will ship with built binaries.

NOTES:
    * Files labeled *.qrc are not parsed unless explicitely set in the parameters list.
    * All *.qmltypes files are ignored, since they are not intended to be useful to the compiler.
    * qmldir files are currently ignored and so import definitions need to be handled accordingly. Would love for this to change.
`

var packageName = flag.String("package", "main", "package name that qrc.go will be under (not needed for go generate)")

// XXX any changes made here should be copied exactly into its counterpart in the template below
func qrcPackResources(subdirs []string) ([]byte, error) {

	type qrcFile struct {
		Alias string        `xml:"alias,attr"`
		Name  string        `xml:",chardata"`
	}

	type qrcResource struct {
		Prefix string        `xml:"prefix,attr"`
		Files  []qrcFile    `xml:"file"`
	}

	type qrcQrcFile struct {
		XMLName   xml.Name      `xml:"RCC"`
		Resources []qrcResource `xml:"qresource"`
	}

	qrcParseQrc := func(name string) (map[string]string, error) {
		data, err := ioutil.ReadFile(name)
		if err != nil {
			return nil, err
		}

		dir := path.Dir(name)

		qrc := qrcQrcFile{}
		err = xml.Unmarshal(data, &qrc)
		if err != nil {
			return nil, err
		}

		out := make(map[string]string)

		for _, resource := range qrc.Resources {
			for _, file := range resource.Files {
				label := file.Name
				if file.Alias != "" {
					label = file.Alias
				}
				out[path.Join(resource.Prefix, label)] = path.Join(dir, file.Name)
			}
		}
		return out, nil
	}

	var rp qml.ResourcesPacker

	for _, subdir := range subdirs {
		err := filepath.Walk(subdir, func(name string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			// ignore qmldir files for now
			if info.Name() == "qmldir" {
				return nil
			}

			ext := path.Ext(name)
			// *.qmltypes files shouldn't be included
			switch ext {
			case ".qmltypes":
				break
			case ".qrc":
				files, err := qrcParseQrc(name)
				if err != nil {
					return err
				}
				for label, filename := range files {
					data, err := ioutil.ReadFile(filename)
					if err != nil {
						return err
					}
					rp.Add(label, data)
				}
			default:
				data, err := ioutil.ReadFile(name)
				if err != nil {
					return err
				}
				rp.Add(filepath.ToSlash(name), data)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return rp.Pack().Bytes(), nil
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s", doc)
		flag.PrintDefaults()
	}
	flag.Parse()
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	subdirs := flag.Args()
	if len(subdirs) == 0 {
		return fmt.Errorf("must provide at least one path")
	}

	resdata, err := qrcPackResources(subdirs)
	if err != nil {
		return err
	}

	f, err := os.Create("qrc.go")
	if err != nil {
		return err
	}
	defer f.Close()

	data := templateData{
		PackageName:   *packageName,
		SubDirs:       subdirs,
		ResourcesData: resdata,
	}

	// $GOPACKAGE is set automatically by go generate.
	if pkgname := os.Getenv("GOPACKAGE"); pkgname != "" {
		data.PackageName = pkgname
	}

	return tmpl.Execute(f, data)
}

type templateData struct {
	PackageName   string
	SubDirs       []string
	ResourcesData []byte
}

func buildTemplate(name, content string) *template.Template {
	return template.Must(template.New(name).Parse(content))
}

var tmpl = buildTemplate("qrc.go", `package {{.PackageName}}

// This file is automatically generated by gopkg.in/qml.v1/cmd/genqrc

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"encoding/xml"

	"gopkg.in/qml.v1"
)

func init() {
	qrcResourcesData := {{printf "%q" .ResourcesData}}

	if os.Getenv("QRC_REPACK") == "1" {
		data, err := qrcPackResources({{printf "%#v" .SubDirs}})
		if err != nil {
			panic("cannot repack qrc resources: " + err.Error())
		}
		qrcResourcesData = string(data)
	}
	r, err := qml.ParseResourcesString(qrcResourcesData)
	if err != nil {
		panic("cannot parse bundled resources data: " + err.Error())
	}
	qml.LoadResources(r)
}

func qrcPackResources(subdirs []string) ([]byte, error) {

	type qrcFile struct {
		Alias string        ` + "`xml:\"alias,attr\"`" + `
		Name  string        ` + "`xml:\",chardata\"`" + `
	}

	type qrcResource struct {
		Prefix string        ` + "`xml:\"prefix,attr\"`" + `
		Files  []qrcFile    ` + "`xml:\"file\"`" + `
	}

	type qrcQrcFile struct {
		XMLName   xml.Name      ` + "`xml:\"RCC\"`" + `
		Resources []qrcResource ` + "`xml:\"qresource\"`" + `
	}

	qrcParseQrc := func(name string) (map[string]string, error) {
		data, err := ioutil.ReadFile(name)
		if err != nil {
			return nil, err
		}

		base := path.Dir(name)

		qrc := qrcQrcFile{}
		err = xml.Unmarshal(data, &qrc)
		if err != nil {
			return nil, err
		}

		out := make(map[string]string)

		for _, resource := range qrc.Resources {
			for _, file := range resource.Files {
				label := file.Name
				if file.Alias != "" {
					label = file.Alias
				}
				out[path.Join(resource.Prefix, label)] = path.Join(base, file.Name)
			}
		}
		return out, nil
	}

	var rp qml.ResourcesPacker

	for _, subdir := range subdirs {
		err := filepath.Walk(subdir, func(name string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			// ignore qmldir files for now
			if info.Name() == "qmldir" {
				return nil
			}

			ext := path.Ext(name)
			// *.qmltypes files shouldn't be included
			switch ext {
			case ".qmltypes":
				break
			case ".qrc":
				files, err := qrcParseQrc(name)
				if err != nil {
					return err
				}
				for label, filename := range files {
					data, err := ioutil.ReadFile(filename)
					if err != nil {
						return err
					}
					rp.Add(label, data)
				}
			default:
				data, err := ioutil.ReadFile(name)
				if err != nil {
					return err
				}
				rp.Add(filepath.ToSlash(name), data)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return rp.Pack().Bytes(), nil
}
`)
