package envconfig

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

var (
	ErrUnexportedField = errors.New("envconfig: unexported field")
)

type context struct {
	name       string
	customName string
	optional   bool
	defaultVal string
}

// Unmarshaler is the interface implemented by objects that can unmarshal
// a environment variable string of themselves.
type Unmarshaler interface {
	Unmarshal(s string) error
}

// Init reads the configuration from environment variables and populates the conf object. conf must be a pointer
func Init(conf interface{}) error {
	return InitWithPrefix(conf, "")
}

// InitWithPrefix reads the configuration from environment variables and populates the conf object. conf must be a pointer.
// Each key read will be prefixed with the prefix string.
func InitWithPrefix(conf interface{}, prefix string) error {
	value := reflect.ValueOf(conf)
	if value.Kind() != reflect.Ptr {
		return errors.New("envconfig: value is not a pointer")
	}

	elem := value.Elem()

	switch elem.Kind() {
	case reflect.Ptr:
		elem.Set(reflect.New(elem.Type().Elem()))
		return readStruct(elem.Elem(), &context{name: prefix})
	case reflect.Struct:
		return readStruct(elem, &context{name: prefix})
	default:
		return errors.New("envconfig: invalid value kind, only works on structs")
	}
}

type tag struct {
	customName string
	optional   bool
	skip       bool
	defaultVal string
}

func parseTag(s string) *tag {
	tag := &tag{}

	tokens := strings.Split(s, ",")
	if len(tokens) == 0 {
		return tag
	}

	for _, v := range tokens {
		switch {
		case v == "-":
			tag.skip = true
		case v == "optional":
			tag.optional = true
		case strings.HasPrefix(v, "default="):
			tag.defaultVal = strings.TrimPrefix(v, "default=")
		default:
			tag.customName = v
		}
	}

	return tag
}

func readStruct(value reflect.Value, ctx *context) (err error) {
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		name := value.Type().Field(i).Name

		tag := parseTag(value.Type().Field(i).Tag.Get("envconfig"))
		if tag.skip {
			continue
		}

	doRead:
		switch field.Kind() {
		case reflect.Ptr:
			// it's a pointer, create a new value and restart the switch
			field.Set(reflect.New(field.Type().Elem()))
			field = field.Elem()
			goto doRead
		case reflect.Struct:
			err = readStruct(field, &context{
				name:       combineName(ctx.name, name),
				optional:   ctx.optional || tag.optional,
				defaultVal: tag.defaultVal,
			})
		default:
			err = setField(field, &context{
				name:       combineName(ctx.name, name),
				customName: tag.customName,
				optional:   ctx.optional || tag.optional,
				defaultVal: tag.defaultVal,
			})
		}

		if err != nil {
			return err
		}
	}

	return
}

var byteSliceType = reflect.TypeOf([]byte(nil))

func setField(value reflect.Value, ctx *context) (err error) {
	str, err := readValue(ctx)
	if err != nil {
		return err
	}
	if len(str) == 0 && ctx.optional {
		return nil
	}

	vkind := value.Kind()
	switch {
	case vkind == reflect.Slice && !isUnmarshaler(value.Type()):
		if value.Type() == byteSliceType {
			return parseBytesValue(value, str)
		}
		return setSliceField(value, str)
	default:
		return parseValue(value, str)
	}
}

func setSliceField(value reflect.Value, str string) error {
	elType := value.Type().Elem()
	tnz := newSliceTokenizer(str)

	slice := reflect.MakeSlice(value.Type(), value.Len(), value.Cap())

	for tnz.scan() {
		token := tnz.text()

		el := reflect.New(elType).Elem()

		if err := parseValue(el, token); err != nil {
			return err
		}

		slice = reflect.Append(slice, el)
	}

	value.Set(slice)

	return tnz.Err()
}

var (
	durationType    = reflect.TypeOf(new(time.Duration)).Elem()
	unmarshalerType = reflect.TypeOf(new(Unmarshaler)).Elem()
)

func isDurationField(t reflect.Type) bool {
	return t.AssignableTo(durationType)
}

func isUnmarshaler(t reflect.Type) bool {
	return t.Implements(unmarshalerType) || reflect.PtrTo(t).Implements(unmarshalerType)
}

func parseValue(v reflect.Value, str string) (err error) {
	if !v.CanSet() {
		return ErrUnexportedField
	}

	vtype := v.Type()

	// Special case for Unmarshaler
	if isUnmarshaler(vtype) {
		return parseWithUnmarshaler(v, str)
	}

	// Special case for time.Duration
	if isDurationField(vtype) {
		return parseDuration(v, str)
	}

	kind := vtype.Kind()
	switch kind {
	case reflect.Bool:
		err = parseBoolValue(v, str)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		err = parseIntValue(v, str)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		err = parseUintValue(v, str)
	case reflect.Float32, reflect.Float64:
		err = parseFloatValue(v, str)
	case reflect.Ptr:
		v.Set(reflect.New(vtype.Elem()))
		return parseValue(v.Elem(), str)
	case reflect.String:
		v.SetString(str)
	case reflect.Struct:
		err = parseStruct(v, str)
	default:
		return fmt.Errorf("envconfig: slice element type %v not supported", kind)
	}

	return
}

func parseWithUnmarshaler(v reflect.Value, str string) error {
	var u Unmarshaler
	vtype := v.Type()

	if vtype.Implements(unmarshalerType) {
		u = v.Interface().(Unmarshaler)
	} else if reflect.PtrTo(vtype).Implements(unmarshalerType) {
		// We know the interface has a pointer receiver, but our value might not be a pointer, so we get one
		if v.Kind() != reflect.Ptr {
			u = v.Addr().Interface().(Unmarshaler)
		} else {
			u = v.Interface().(Unmarshaler)
		}
	}

	return u.Unmarshal(str)
}

func parseDuration(v reflect.Value, str string) error {
	d, err := time.ParseDuration(str)
	if err != nil {
		return err
	}

	v.SetInt(int64(d))

	return nil
}

func parseStruct(value reflect.Value, token string) error {
	tokens := strings.Split(token[1:len(token)-1], ",")
	if len(tokens) == 0 {
		return errors.New("envconfig: struct token should not be empty")
	}

	if len(tokens) != value.NumField() {
		return fmt.Errorf("envconfig: struct token has %d fields but struct has %d",
			len(tokens), value.NumField(),
		)
	}

	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		t := tokens[i]

		if err := parseValue(field, t); err != nil {
			return err
		}
	}

	return nil
}

func parseBoolValue(v reflect.Value, str string) error {
	val, err := strconv.ParseBool(str)
	if err != nil {
		return err
	}
	v.SetBool(val)

	return nil
}

func parseIntValue(v reflect.Value, str string) error {
	val, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return err
	}
	v.SetInt(val)

	return nil
}

func parseUintValue(v reflect.Value, str string) error {
	val, err := strconv.ParseUint(str, 10, 64)
	if err != nil {
		return err
	}
	v.SetUint(val)

	return nil
}

func parseFloatValue(v reflect.Value, str string) error {
	val, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return err
	}
	v.SetFloat(val)

	return nil
}

func parseBytesValue(v reflect.Value, str string) error {
	val, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return err
	}
	v.SetBytes(val)

	return nil
}

func combineName(parentName, name string) string {
	if parentName == "" {
		return name
	}

	return parentName + "." + name
}

func readValue(ctx *context) (string, error) {
	var key string
	if ctx.customName == "" {
		key = strings.Replace(ctx.name, ".", "_", -1)
		key = strings.ToUpper(key)
	} else {
		key = ctx.customName
	}

	str := os.Getenv(key)
	if str != "" {
		return str, nil
	}

	if ctx.defaultVal != "" {
		return ctx.defaultVal, nil
	}

	if ctx.optional {
		return "", nil
	}

	return "", fmt.Errorf("envconfig: key %v not found", key)
}
