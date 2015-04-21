package envconfig

import (
	"errors"
	"fmt"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// EnvUnmarshaler is the interface implemented by objects that can unmarshal
// a environment variable string of themselves.
type EnvUnmarshaler interface {
	Unmarshal(s string) error
}

// Init reads the configuration from environment variables and populates the conf object
func Init(conf interface{}) error {
	value := reflect.ValueOf(conf)
	if value.Kind() != reflect.Ptr {
		return errors.New("envconfig: value is not a pointer")
	}

	elem := value.Elem()

	switch elem.Kind() {
	case reflect.Ptr:
		elem.Set(reflect.New(elem.Type().Elem()))
		return readStruct(elem.Elem(), "")
	case reflect.Struct:
		return readStruct(elem, "")
	default:
		return errors.New("envconfig: invalid value kind, only works on structs")
	}
}

func readStruct(value reflect.Value, parentName string) (err error) {
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		name := value.Type().Field(i).Name
		combinedName := combineName(parentName, name)

	doRead:
		switch field.Kind() {
		case reflect.Ptr:
			// it's a pointer, create a new value and restart the switch
			field.Set(reflect.New(field.Type().Elem()))
			field = field.Elem()
			goto doRead
		case reflect.Struct:
			err = readStruct(field, combinedName)
		default:
			err = setField(field, combinedName)
		}

		if err != nil {
			return err
		}
	}

	return
}

func setField(value reflect.Value, name string) (err error) {
	r := reader{}
	// TODO(vincent): optional field
	r.readValue(name, false)
	if r.err != nil {
		return r.err
	}

	switch value.Kind() {
	case reflect.Slice:
		return setSliceField(value, r.str)
	default:
		return parseValue(value, r.str)
	}

	return
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
	unmarshalerType = reflect.TypeOf(new(EnvUnmarshaler)).Elem()
)

func isDurationField(t reflect.Type) bool {
	return t.AssignableTo(durationType)
}

func isUnmarshaler(t reflect.Type) bool {
	return t.Implements(unmarshalerType) || reflect.PtrTo(t).Implements(unmarshalerType)
}

func parseValue(v reflect.Value, str string) (err error) {
	vtype := v.Type()

	// Special case for EnvUnmarshaler
	if isUnmarshaler(vtype) {
		return parseWithEnvUnmarshaler(v, str)
	}

	// Special case for time.Duration
	if isDurationField(vtype) {
		return parseDuration(v, str)
	}

	kind := v.Type().Kind()
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

func parseWithEnvUnmarshaler(v reflect.Value, str string) error {
	var u EnvUnmarshaler
	vtype := v.Type()

	if vtype.Implements(unmarshalerType) {
		u = v.Interface().(EnvUnmarshaler)
	} else if reflect.PtrTo(vtype).Implements(unmarshalerType) {
		// We know the interface has a pointer receiver, but our value might not be a pointer, so we get one
		if v.Kind() != reflect.Ptr {
			u = v.Addr().Interface().(EnvUnmarshaler)
		} else {
			u = v.Interface().(EnvUnmarshaler)
		}
	}

	return u.Unmarshal(str)
}

func parseDuration(v reflect.Value, str string) error {
	d, err := time.ParseDuration(str)
	if err != nil {
		return nil
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

func nameToKey(name string) string {
	s := strings.Replace(name, ".", "_", -1)
	s = strings.ToUpper(s)

	return s
}

func combineName(parentName, name string) string {
	if parentName == "" {
		return name
	}

	return parentName + "." + name
}

type reader struct {
	err error
	str string
}

func (r *reader) readValue(name string, optional bool) {
	if r.err != nil {
		return
	}

	key := nameToKey(name)
	r.str = os.Getenv(key)
	if r.str != "" {
		return
	}

	if optional {
		return
	}

	r.err = fmt.Errorf("envconfig: key %v not found", key)
}

func (r *reader) toInt64() (int64, error) {
	if r.err != nil {
		return -1, r.err
	}

	return strconv.ParseInt(r.str, 10, 64)
}

func (r *reader) toUint64() (uint64, error) {
	if r.err != nil {
		return 0, r.err
	}

	return strconv.ParseUint(r.str, 10, 64)
}

func (r *reader) toBool() (bool, error) {
	if r.err != nil {
		return false, r.err
	}

	return strconv.ParseBool(r.str)
}

func (r *reader) toFloat64() (float64, error) {
	if r.err != nil {
		return math.NaN(), r.err
	}

	return strconv.ParseFloat(r.str, 64)
}
