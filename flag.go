package ask

import (
	"flag"
	"fmt"
	"reflect"
	"unsafe"
)

type Flag struct {
	Value flag.Value
	// Name, full version of the flag (or shorthand if no full version),
	// excluding '-' or '--'.
	Name string
	// Shorthand, single character version of the flag, excluding '-'.
	// 0 if no shorthand.
	Shorthand uint8
	// Env is the key used to load this as env variable.
	// If unspecified, an env var is inferred from the full flag path (see FlagPathToEnvKey).
	// If set to "-", this flag cannot be set using an env var.
	Env      string
	IsArg    bool
	Help     string
	Default  string
	Required bool
	// Deprecated states the reason for deprecation. Empty if not deprecated.
	Deprecated string
	Hidden     bool
}

type PrefixedFlag struct {
	// Path combines prefix and flag name, segments separated by dot
	Path string
	*Flag
}

func FlagValue(typ reflect.Type, val reflect.Value) (flag.Value, error) {
	// Get the pointer to the destination struct, to route pflags to
	ptr := unsafe.Pointer(val.Addr().Pointer())

	var fl flag.Value

	if typ.Implements(typedFlagValueType) {
		fl = val.Interface().(TypedValue)
	} else if reflect.PtrTo(typ).Implements(typedFlagValueType) {
		fl = val.Addr().Interface().(TypedValue)
	} else if typ.Implements(flagValueType) {
		fl = val.Interface().(flag.Value)
	} else if reflect.PtrTo(typ).Implements(flagValueType) {
		fl = val.Addr().Interface().(flag.Value)
	} else if typ == durationType {
		fl = (*DurationValue)(ptr)
	} else if typ == ipType {
		fl = (*IPValue)(ptr)
	} else if typ == ipNetType {
		fl = (*IPNetValue)(ptr)
	} else if typ == ipmaskType {
		fl = (*IPMaskValue)(ptr)
	} else {
		switch typ.Kind() {
		// unsigned integers
		case reflect.Uint:
			fl = (*UintValue)(ptr)
		case reflect.Uint8:
			fl = (*Uint8Value)(ptr)
		case reflect.Uint16:
			fl = (*Uint16Value)(ptr)
		case reflect.Uint32:
			fl = (*Uint32Value)(ptr)
		case reflect.Uint64:
			fl = (*Uint64Value)(ptr)
		// signed integers
		case reflect.Int:
			fl = (*IntValue)(ptr)
		case reflect.Int8:
			fl = (*Int8Value)(ptr)
		case reflect.Int16:
			fl = (*Int16Value)(ptr)
		case reflect.Int32:
			fl = (*Int32Value)(ptr)
		case reflect.Int64:
			fl = (*Int64Value)(ptr)
		// Misc
		case reflect.String:
			fl = (*StringValue)(ptr)
		case reflect.Bool:
			fl = (*BoolValue)(ptr)
		case reflect.Float32:
			fl = (*Float32Value)(ptr)
		case reflect.Float64:
			fl = (*Float64Value)(ptr)
		// Cobra commons
		case reflect.Slice:
			elemTyp := typ.Elem()
			if elemTyp == durationType {
				fl = (*DurationSliceValue)(ptr)
			} else if elemTyp == ipType {
				fl = (*IPSliceValue)(ptr)
			} else {
				switch elemTyp.Kind() {
				case reflect.Array:
					switch elemTyp.Elem().Kind() {
					case reflect.Uint8:
						fl = &fixedLenBytesSlice{Dest: val}
					default:
						return nil, fmt.Errorf("unrecognized element type of array-element slice: %v", elemTyp.Elem().String())
					}
				case reflect.Uint8:
					b := (*[]byte)(ptr)
					fl = (*BytesHexFlag)(b)
				case reflect.Uint16:
					fl = (*Uint16SliceValue)(ptr)
				case reflect.Uint32:
					fl = (*Uint32SliceValue)(ptr)
				case reflect.Uint64:
					fl = (*Uint64SliceValue)(ptr)
				case reflect.Uint:
					fl = (*UintSliceValue)(ptr)
				case reflect.Int8:
					fl = (*Int8SliceValue)(ptr)
				case reflect.Int16:
					fl = (*Int16SliceValue)(ptr)
				case reflect.Int32:
					fl = (*Int32SliceValue)(ptr)
				case reflect.Int64:
					fl = (*Int64SliceValue)(ptr)
				case reflect.Int:
					fl = (*IntSliceValue)(ptr)
				case reflect.Float32:
					fl = (*Float32SliceValue)(ptr)
				case reflect.Float64:
					fl = (*Float64SliceValue)(ptr)
				case reflect.String:
					fl = (*StringSliceValue)(ptr)
				case reflect.Bool:
					fl = (*BoolSliceValue)(ptr)
				default:
					return nil, fmt.Errorf("unrecognized slice element type: %v", elemTyp.String())
				}
			}
		case reflect.Array:
			elemTyp := typ.Elem()
			switch elemTyp.Kind() {
			case reflect.Uint8:
				expectedLen := val.Len()
				destSlice := val.Slice(0, expectedLen).Bytes()
				fl = &fixedLenBytes{
					Dest:           destSlice,
					ExpectedLength: uint64(expectedLen),
				}
			default:
				return nil, fmt.Errorf("unrecognized array element type: %v", elemTyp.String())
			}
		case reflect.Ptr:
			contentTyp := typ.Elem()
			// allocate a destination value if it doesn't exist yet
			if val.IsNil() {
				val.Set(reflect.New(contentTyp))
			}
			// and recurse into the type
			return FlagValue(typ.Elem(), val.Elem())
		default:
			return nil, fmt.Errorf("unrecognized type: %v", typ.String())
		}
	}
	return fl, nil
}
