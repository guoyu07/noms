package nomgen

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"strings"
	"text/template"

	. "github.com/attic-labs/noms/dbg"
	"github.com/attic-labs/noms/types"
)

var (
	fieldTempl  = readTemplate("field.tmpl")
	headerTmpl  = readTemplate("header.tmpl")
	listTempl   = readTemplate("list.tmpl")
	mapTempl    = readTemplate("map.tmpl")
	setTempl    = readTemplate("set.tmpl")
	structTempl = readTemplate("struct.tmpl")
)

type NG struct {
	w       io.Writer
	written types.Set
	toWrite types.Set
}

func New(w io.Writer) NG {
	return NG{w: w, written: types.NewSet(), toWrite: types.NewSet()}
}

func (ng *NG) WriteGo(val types.Map, pkg string) {
	headerTmpl.Execute(ng.w, struct{ PackageName string }{pkg})

	ng.addType(val)

	for !ng.toWrite.Empty() {
		t := ng.toWrite.Any()
		ng.toWrite = ng.toWrite.Remove(t)
		ng.written = ng.written.Insert(t)
		ng.writeType(t.(types.Map))
	}
}

func (ng *NG) addType(val types.Value) {
	switch val := val.(type) {
	case types.String:
		// Nothing to do, the type is primitive
		return
	case types.Map:
		if ng.written.Has(val) || ng.toWrite.Has(val) {
			return
		}
		ng.toWrite = ng.toWrite.Insert(val)
	default:
		Chk.Fail(fmt.Sprintf("Unexpected typedef: %+v", val))
	}
}

func fromNomsValue(name string) string {
	if name == "types.Value" {
		return ""
	}
	return fmt.Sprintf("%sFromVal", name)
}

func toNomsValue(name string) string {
	if strings.HasPrefix(name, "types.") {
		return ""
	}
	return ".NomsValue()"
}

func readTemplate(name string) *template.Template {
	_, thisfile, _, _ := runtime.Caller(1)
	f, err := os.Open(path.Join(path.Dir(thisfile), name))
	Chk.NoError(err)
	defer f.Close()
	b, err := ioutil.ReadAll(f)
	Chk.NoError(err)
	t, err := template.New(name).Funcs(template.FuncMap{
		"fromVal": fromNomsValue,
		"toVal":   toNomsValue,
	}).Parse(string(b))
	Chk.NoError(err)
	return t
}

func (ng *NG) writeType(val types.Map) {
	typ := val.Get(types.NewString("$type")).(types.String).String()
	switch typ {
	case "noms.ListDef":
		ng.writeList(val)
		return
	case "noms.MapDef":
		ng.writeMap(val)
		return
	case "noms.SetDef":
		ng.writeSet(val)
		return
	case "noms.StructDef":
		ng.writeStruct(val)
		return
	}
	Chk.Fail(fmt.Sprintf("Unexpected typedef: %+v", val))
}

func (ng *NG) writeSet(val types.Map) {
	elem := val.Get(types.NewString("elem"))
	ng.addType(elem)

	data := struct {
		StructName string
		ElemName   string
	}{
		getGoTypeName(val),
		getGoTypeName(elem),
	}

	setTempl.Execute(ng.w, data)
}

func (ng *NG) writeList(val types.Map) {
	elem := val.Get(types.NewString("elem"))
	ng.addType(elem)

	data := struct {
		StructName string
		ElemName   string
	}{
		getGoTypeName(val),
		getGoTypeName(elem),
	}

	listTempl.Execute(ng.w, data)
}

func (ng *NG) writeMap(val types.Map) {
	key := val.Get(types.NewString("key"))
	ng.addType(key)
	valueName := val.Get(types.NewString("value"))
	ng.addType(valueName)

	data := struct {
		StructName string
		KeyName    string
		ValueName  string
	}{
		getGoTypeName(val),
		getGoTypeName(key),
		getGoTypeName(valueName),
	}

	mapTempl.Execute(ng.w, data)
}

func (ng *NG) writeStruct(val types.Map) {
	structName := getGoTypeName(val)
	structTempl.Execute(ng.w, struct {
		StructName string
	}{
		getGoTypeName(val),
	})

	val.Iter(func(k, v types.Value) (stop bool) {
		sk := k.(types.String).String()
		if sk[0] != '$' {
			ng.writeField(structName, sk, v)
		}
		return
	})
}

func (ng *NG) writeField(structName, fieldName string, typeDef types.Value) {
	ng.addType(typeDef)

	data := struct {
		StructName  string
		FieldType   string
		GoFieldName string
		FieldName   string
	}{
		structName,
		getGoTypeName(typeDef),
		strings.Title(fieldName),
		fieldName,
	}

	fieldTempl.Execute(ng.w, data)
}

func getGoTypeName(typeDef types.Value) string {
	typeName := getGoStructName(typeDef)
	switch typeDef.(type) {
	case types.String:
		return fmt.Sprintf("types.%s", typeName)
	}
	return typeName
}

func getGoStructName(typeDef types.Value) string {
	switch typeDef := typeDef.(type) {
	case types.String:
		name := typeDef.String()
		switch name {
		case "bool", "int16", "int32", "int64", "uint16", "uint32", "uint64", "float32", "float64", "blob", "string", "set", "map", "value":
			return strings.Title(typeDef.String())
		}
		Chk.Fail("unexpected noms type name: %s", name)
	case types.Map:
		typ := typeDef.Get(types.NewString("$type")).(types.String).String()
		switch typ {
		case "noms.ListDef":
			return fmt.Sprintf("%sList", getGoStructName(typeDef.Get(types.NewString("elem"))))
		case "noms.MapDef":
			return fmt.Sprintf("%s%sMap",
				getGoStructName(typeDef.Get(types.NewString("key"))),
				getGoStructName(typeDef.Get(types.NewString("value"))))
		case "noms.SetDef":
			return fmt.Sprintf("%sSet", getGoStructName(typeDef.Get(types.NewString("elem"))))
		case "noms.StructDef":
			return typeDef.Get(types.NewString("$name")).(types.String).String()
		}
	}
	Chk.Fail("Unexpected typeDef struct: %+v", typeDef)
	return ""
}

func (ng *NG) writeStr(str string, vals ...interface{}) {
	io.WriteString(ng.w, fmt.Sprintf(str, vals...))
}