package envconfig

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var (
	// ErrUnexportedField is the error returned by the Init* functions when a field of the config struct is not exported and the option AllowUnexported is not used.
	ErrUnexportedField = errors.New("envconfig: unexported field")
	// ErrNotAPointer is the error returned by the Init* functions when the configuration object is not a pointer.
	ErrNotAPointer = errors.New("envconfig: value is not a pointer")
	// ErrInvalidValueKind is the error returned by the Init* functions when the configuration object is not a struct.
	ErrInvalidValueKind = errors.New("envconfig: invalid value kind, only works on structs")
)

type context struct {
	name               string
	customName         string
	defaultVal         string
	usingDefault       bool
	parents            []reflect.Value
	optional, leaveNil bool
	allowUnexported    bool
}

// Unmarshaler is the interface implemented by objects that can unmarshal
// a environment variable string of themselves.
type Unmarshaler interface {
	Unmarshal(s string) error
}

// Options is used to customize the behavior of envconfig. Use it with InitWithOptions.
type Options struct {
	// Prefix allows specifying a prefix for each key.
	Prefix string

	// AllOptional determines whether to not throw errors by default for any key
	// that is not found. AllOptional=true means errors will not be thrown.
	AllOptional bool

	// LeaveNil specifies whether to not create new pointers for any pointer fields
	// found within the passed config. Rather, it behaves such that if and only if
	// there is a) a non-empty field in the value or b) a non-empty value that
	// the pointer is pointing to will a new pointer be created. By default,
	// LeaveNil=false will create all pointers in all structs if they are nil.
	//
	//	var X struct {
	//		A *struct{
	//			B string
	//		}
	//	}
	//	envconfig.InitWithOptions(&X, Options{LeaveNil: true})
	//
	// $ ./program
	//
	// X.A == nil
	//
	// $ A_B="string" ./program
	//
	// X.A.B="string" // A will not be nil
	LeaveNil bool

	// AllowUnexported allows unexported fields to be present in the passed config.
	AllowUnexported bool
}

// Init reads the configuration from environment variables and populates the conf object. conf must be a pointer
func Init(conf interface{}) error {
	return InitWithOptions(conf, Options{})
}

// InitWithPrefix reads the configuration from environment variables and populates the conf object. conf must be a pointer.
// Each key read will be prefixed with the prefix string.
func InitWithPrefix(conf interface{}, prefix string) error {
	return InitWithOptions(conf, Options{Prefix: prefix})
}

// InitWithOptions reads the configuration from environment variables and populates the conf object.
// conf must be a pointer.
func InitWithOptions(conf interface{}, opts Options) error {
	value := reflect.ValueOf(conf)
	if value.Kind() != reflect.Ptr {
		return ErrNotAPointer
	}

	elem := value.Elem()

	ctx := context{
		name:            opts.Prefix,
		optional:        opts.AllOptional,
		leaveNil:        opts.LeaveNil,
		allowUnexported: opts.AllowUnexported,
	}
	switch elem.Kind() {
	case reflect.Ptr:
		if elem.IsNil() {
			elem.Set(reflect.New(elem.Type().Elem()))
		}
		_, err := readStruct(elem.Elem(), &ctx)
		return err
	case reflect.Struct:
		_, err := readStruct(elem, &ctx)
		return err
	default:
		return ErrInvalidValueKind
	}
}

type tag struct {
	customName string
	optional   bool
	skip       bool
	defaultVal string
}

func parseTag(s string) *tag {
	var t tag

	tokens := strings.Split(s, ",")
	for _, v := range tokens {
		switch {
		case v == "-":
			t.skip = true
		case v == "optional":
			t.optional = true
		case strings.HasPrefix(v, "default="):
			t.defaultVal = strings.TrimPrefix(v, "default=")
		default:
			t.customName = v
		}
	}

	return &t
}

func readStruct(value reflect.Value, ctx *context) (nonNil bool, err error) {
	var parents []reflect.Value

	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		fieldType := field.Type()
		fieldInfo := value.Type().Field(i)
		name := fieldInfo.Name

		tag := parseTag(fieldInfo.Tag.Get("envconfig"))
		if tag.skip || !field.CanSet() {
			if !field.CanSet() && !ctx.allowUnexported {
				return false, fmt.Errorf("%w %q", ErrUnexportedField, name)
			}
			continue
		}

		parents = ctx.parents

	doRead:
		switch {
		case field.Kind() == reflect.Ptr && !isUnmarshaler(fieldType):
			// it's a pointer, create a new value and restart the switch
			if field.IsNil() {
				field.Set(reflect.New(field.Type().Elem()))
				parents = append(parents, field) // track parent pointers to deallocate if no children are filled in
			}
			field = field.Elem()
			goto doRead
		case field.Kind() == reflect.Struct && !isUnmarshaler(fieldType):
			var nonNilIn bool
			nonNilIn, err = readStruct(field, &context{
				name:            combineName(ctx.name, name),
				optional:        ctx.optional || tag.optional,
				defaultVal:      tag.defaultVal,
				parents:         parents,
				leaveNil:        ctx.leaveNil,
				allowUnexported: ctx.allowUnexported,
			})
			nonNil = nonNil || nonNilIn
		default:
			var ok bool
			ok, err = setField(field, &context{
				name:            combineName(ctx.name, name),
				customName:      tag.customName,
				optional:        ctx.optional || tag.optional,
				defaultVal:      tag.defaultVal,
				parents:         parents,
				leaveNil:        ctx.leaveNil,
				allowUnexported: ctx.allowUnexported,
			})
			nonNil = nonNil || ok
		}

		if err != nil {
			return false, err
		}
	}

	if !nonNil && ctx.leaveNil { // re-zero
		for _, p := range parents {
			p.Set(reflect.Zero(p.Type()))
		}
	}

	return nonNil, err
}

var byteSliceType = reflect.TypeOf([]byte(nil))

func setField(value reflect.Value, ctx *context) (ok bool, err error) {
	str, err := readValue(ctx)
	if err != nil {
		return false, err
	}

	if len(str) == 0 && ctx.optional {
		return false, nil
	}

	isSliceNotUnmarshaler := value.Kind() == reflect.Slice && !isUnmarshaler(value.Type())
	switch {
	case isSliceNotUnmarshaler && value.Type() == byteSliceType:
		err := parseBytesValue(value, str)
		if err != nil {
			err = fmt.Errorf("envconfig: unable to parse value %q as bytes for possible keys %v. err=%v", str, makeAllPossibleKeys(ctx), err)
		}
		return true, err

	case isSliceNotUnmarshaler:
		return true, setSliceField(value, str, ctx)

	default:
		return true, parseValue(value, str, ctx)
	}
}

func setSliceField(value reflect.Value, str string, ctx *context) error {
	separator := sliceEnvSeparator
	if ctx.usingDefault {
		separator = sliceDefaultSeparator
	}

	elType := value.Type().Elem()
	tnz := newSliceTokenizer(str, separator)

	slice := reflect.MakeSlice(value.Type(), 0, value.Cap())

	for tnz.scan() {
		token := tnz.text()

		el := reflect.New(elType).Elem()

		if err := parseValue(el, token, ctx); err != nil {
			return err
		}

		slice = reflect.Append(slice, el)
	}

	value.Set(slice)

	return tnz.Err()
}

var (
	durationType    = reflect.TypeOf((*time.Duration)(nil)).Elem()
	unmarshalerType = reflect.TypeOf((*Unmarshaler)(nil)).Elem()
)

func isDurationField(t reflect.Type) bool {
	return t.AssignableTo(durationType)
}

func isUnmarshaler(t reflect.Type) bool {
	return t.Implements(unmarshalerType) || reflect.PtrTo(t).Implements(unmarshalerType)
}

func parseValue(v reflect.Value, str string, ctx *context) (err error) {
	vtype := v.Type()

	// Special case when the type is a map: we need to make the map
	switch vtype.Kind() {
	case reflect.Map:
		v.Set(reflect.MakeMap(vtype))
	}

	kind := vtype.Kind()
	switch {
	case isUnmarshaler(vtype):
		// Special case for Unmarshaler
		err = parseWithUnmarshaler(v, str)
	case isDurationField(vtype):
		// Special case for time.Duration
		err = parseDuration(v, str)
	case kind == reflect.Bool:
		err = parseBoolValue(v, str)
	case kind == reflect.Int, kind == reflect.Int8, kind == reflect.Int16, kind == reflect.Int32, kind == reflect.Int64:
		err = parseIntValue(v, str)
	case kind == reflect.Uint, kind == reflect.Uint8, kind == reflect.Uint16, kind == reflect.Uint32, kind == reflect.Uint64:
		err = parseUintValue(v, str)
	case kind == reflect.Float32, kind == reflect.Float64:
		err = parseFloatValue(v, str)
	case kind == reflect.Ptr:
		v.Set(reflect.New(vtype.Elem()))
		return parseValue(v.Elem(), str, ctx)
	case kind == reflect.String:
		v.SetString(str)
	case kind == reflect.Struct:
		err = parseStruct(v, str, ctx)
	default:
		return fmt.Errorf("envconfig: kind %v not supported", kind)
	}

	if err != nil {
		return fmt.Errorf("envconfig: unable to parse value %q for possible keys %v. err=%v", str, makeAllPossibleKeys(ctx), err)
	}

	return
}

func parseWithUnmarshaler(v reflect.Value, str string) error {
	var u Unmarshaler
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		u = v.Interface().(Unmarshaler)
	} else {
		u = v.Addr().Interface().(Unmarshaler)
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

// NOTE(vincent): this is only called when parsing structs inside a slice.
func parseStruct(value reflect.Value, token string, ctx *context) error {
	separator := string(sliceEnvSeparator)
	if ctx.usingDefault {
		separator = string(sliceDefaultSeparator)
	}

	tokens := strings.Split(token[1:len(token)-1], separator)
	if len(tokens) != value.NumField() {
		return fmt.Errorf("struct token has %d fields but struct has %d", len(tokens), value.NumField())
	}

	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		t := tokens[i]

		if err := parseValue(field, t, ctx); err != nil {
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
	keys := makeAllPossibleKeys(ctx)

	var str string

	for _, key := range keys {
		str = os.Getenv(key)
		if str != "" {
			break
		}
	}

	if str != "" {
		return str, nil
	}

	if ctx.defaultVal != "" {
		ctx.usingDefault = true
		return ctx.defaultVal, nil
	}

	if ctx.optional {
		return "", nil
	}

	return "", fmt.Errorf("envconfig: keys %s not found", strings.Join(keys, ", "))
}

func makeAllPossibleKeys(ctx *context) (res []string) {
	if ctx.customName != "" {
		return []string{ctx.customName}
	}

	tmp := make(map[string]struct{})
	{
		n := []rune(ctx.name)

		var buf bytes.Buffer  // this is the buffer where we put extra underscores on "word" boundaries
		var buf2 bytes.Buffer // this is the buffer with the standard naming scheme

		wroteUnderscore := false
		for i, r := range ctx.name {
			if r == '.' {
				buf.WriteRune('_')
				buf2.WriteRune('_')
				wroteUnderscore = true
				continue
			}

			prevOrNextLower := i+1 < len(n) && i-1 > 0 && (unicode.IsLower(n[i+1]) || unicode.IsLower(n[i-1]))
			if i > 0 && unicode.IsUpper(r) && prevOrNextLower && !wroteUnderscore {
				buf.WriteRune('_')
			}

			buf.WriteRune(r)
			buf2.WriteRune(r)

			wroteUnderscore = false
		}

		tmp[strings.ToLower(buf.String())] = struct{}{}
		tmp[strings.ToUpper(buf.String())] = struct{}{}
		tmp[strings.ToLower(buf2.String())] = struct{}{}
		tmp[strings.ToUpper(buf2.String())] = struct{}{}
	}

	for k := range tmp {
		res = append(res, k)
	}

	sort.Strings(res)

	return
}
