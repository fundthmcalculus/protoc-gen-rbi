package ruby_types

import (
	"fmt"
	"log"
	"strings"

	pgs "github.com/lyft/protoc-gen-star"
)

type methodType int

const (
	methodTypeGetter methodType = iota
	methodTypeSetter
	methodTypeInitializer
)

// FieldType intersection between pgs.FieldType and pgs.FieldTypeElem
type FieldType interface {
	ProtoType() pgs.ProtoType
	IsEmbed() bool
	IsEnum() bool
	Imports() []pgs.File
	Enum() pgs.Enum
	Embed() pgs.Message
}

// EntityWithParent intersection between pgs.Message and pgs.Enum
type EntityWithParent interface {
	pgs.Entity
	Parent() pgs.ParentEntity
}

func RubyModules(file pgs.File) []string {
	p := RubyPackage(file)
	split := strings.Split(p, "::")
	modules := make([]string, 0)
	for i := 0; i < len(split); i++ {
		modules = append(modules, strings.Join(split[0:i+1], "::"))
	}
	return modules
}

func RubyPackage(file pgs.File) string {
	pkg := file.Descriptor().GetOptions().GetRubyPackage()
	if pkg == "" {
		pkg = file.Descriptor().GetPackage()
	}
	pkg = strings.Replace(pkg, ".", "::", -1)
	// right now the ruby_out doesn't camelcase the ruby_package, but this results in invalid classes, so do it:
	return upperCamelCase(pkg)
}

func RubyMessageType(entity EntityWithParent) string {
	names := make([]string, 0)
	outer := entity
	ok := true
	for ok {
		name := outer.Name().String()
		names = append([]string{strings.Title(name)}, names...)
		outer, ok = outer.Parent().(pgs.Message)
	}
	return fmt.Sprintf("%s::%s", RubyPackage(entity.File()), strings.Join(names, "::"))
}

func RbsGetterFieldType(field pgs.Field) string {
	return rbsFieldType(field, methodTypeGetter)
}

func RbsSetterFieldType(field pgs.Field) string {
	return rbsFieldType(field, methodTypeSetter)
}

func RbsInitializerFieldType(field pgs.Field) string {
	return rbsFieldType(field, methodTypeInitializer)
}

func rbsFieldType(field pgs.Field, mt methodType) string {
	var rbsType string

	t := field.Type()

	if t.IsMap() {
		rbsType = rbsFieldMapType(field, t, mt)
	} else if t.IsRepeated() {
		rbsType = rbsFieldRepeatedType(field, t, mt)
	} else {
		rbsType = rbsProtoTypeElem(field, t, mt)
	}

	// initializer fields can be passed a `nil` value for all field types
	// messages are already wrapped so we skip those
	if mt == methodTypeInitializer && (t.IsMap() || t.IsRepeated() || t.ProtoType() != pgs.MessageT) {
		return fmt.Sprintf("%s?", rbsType)
	}

	return rbsType
}

func rbsFieldMapType(field pgs.Field, ft pgs.FieldType, mt methodType) string {
	if mt == methodTypeSetter {
		return "Google::Protobuf::Map"
	}
	key := rbsProtoTypeElem(field, ft.Key(), mt)
	value := rbsProtoTypeElem(field, ft.Element(), mt)
	return fmt.Sprintf("::Hash[%s, %s]", key, value)
}

func rbsFieldRepeatedType(field pgs.Field, ft pgs.FieldType, mt methodType) string {
	// An enumerable/array is not accepted at the setter
	// See: https://github.com/protocolbuffers/protobuf/issues/4969
	// See: https://developers.google.com/protocol-buffers/docs/reference/ruby-generated#repeated-fields
	if mt == methodTypeSetter {
		return "Google::Protobuf::RepeatedField"
	}
	value := rbsProtoTypeElem(field, ft.Element(), mt)
	return fmt.Sprintf("Array[%s]", value)
}

func RubyFieldValue(field pgs.Field) string {
	t := field.Type()
	if t.IsMap() {
		key := rubyMapType(t.Key())
		if t.Element().ProtoType() == pgs.MessageT {
			value := RubyMessageType(t.Element().Embed())
			return fmt.Sprintf("Google::Protobuf::Map.new(%s, :message, %s)", key, value)
		}
		value := rubyMapType(t.Element())
		return fmt.Sprintf("Google::Protobuf::Map.new(%s, %s)", key, value)
	} else if t.IsRepeated() {
		return "[]"
	}
	return rubyProtoTypeValue(field, t)
}

func rbsProtoTypeElem(field pgs.Field, ft FieldType, mt methodType) string {
	pt := ft.ProtoType()
	if pt.IsInt() {
		return "Integer"
	}
	if pt.IsNumeric() {
		return "Float"
	}
	if pt == pgs.StringT || pt == pgs.BytesT {
		return "String"
	}
	if pt == pgs.BoolT {
		return "bool"
	}
	if pt == pgs.EnumT {
		if mt == methodTypeGetter {
			return "Symbol"
		}
		return "(Symbol | String | Integer)"
	}
	if pt == pgs.MessageT {
		return fmt.Sprintf("%s?", RubyMessageType(ft.Embed()))
	}
	log.Panicf("Unsupported field type for field: %v\n", field.Name().String())
	return ""
}

func rubyProtoTypeValue(field pgs.Field, ft FieldType) string {
	pt := ft.ProtoType()
	if pt.IsInt() {
		return "0"
	}
	if pt.IsNumeric() {
		return "0.0"
	}
	if pt == pgs.StringT || pt == pgs.BytesT {
		return "\"\""
	}
	if pt == pgs.BoolT {
		return "false"
	}
	if pt == pgs.EnumT {
		return fmt.Sprintf(":%s", ft.Enum().Values()[0].Name().String())
	}
	if pt == pgs.MessageT {
		return "nil"
	}
	log.Panicf("Unsupported field type for field: %v\n", field.Name().String())
	return ""
}

func rubyMapType(ft FieldType) string {
	switch ft.ProtoType() {
	case pgs.DoubleT:
		return ":double"
	case pgs.FloatT:
		return ":float"
	case pgs.Int64T:
		return ":int64"
	case pgs.UInt64T:
		return ":uint64"
	case pgs.Int32T:
		return ":int32"
	case pgs.Fixed64T:
		return ":fixed64"
	case pgs.Fixed32T:
		return ":fixed32"
	case pgs.BoolT:
		return ":bool"
	case pgs.StringT:
		return ":string"
	case pgs.BytesT:
		return ":bytes"
	case pgs.UInt32T:
		return ":uint32"
	case pgs.EnumT:
		return ":enum"
	case pgs.SFixed32:
		return ":sfixed32"
	case pgs.SFixed64:
		return ":sfixed64"
	case pgs.SInt32:
		return ":sint32"
	case pgs.SInt64:
		return ":sint64"
	}
	log.Panicf("Unsupported map field type\n")
	return ""
}

func RbsMethodParamType(method pgs.Method) string {
	return rbsMethodType(method.Input(), method.ClientStreaming())
}

func RbsMethodReturnType(method pgs.Method) string {
	return rbsMethodType(method.Output(), method.ServerStreaming())
}

func rbsMethodType(message pgs.Message, streaming bool) string {
	t := RubyMessageType(message)
	if streaming {
		return fmt.Sprintf("Enumerable[%s]", t)
	}
	return t
}
