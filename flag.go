package ask

import (
	"flag"
	"fmt"
	"reflect"
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
	Env string
	// IsArg marks a positional arg. It can also be set as named flag (by path) or env var,
	// in which case it is not loaded positionally.
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

// The built-in flag values are named types (e.g. UintValue) with the same underlying type
// as the field types they support (e.g. uint). A pointer to a field is converted to a pointer
// of the flag value type, to read and write the field through the flag.Value interface.
// This is a regular Go pointer conversion, checked by reflection, and also allows named
// field types with a matching underlying type (e.g. `type Port uint16`).
var (
	// typeAliases maps exact field types to the flag value pointer type that can alias them.
	typeAliases = map[reflect.Type]reflect.Type{
		durationType: reflect.TypeOf((*DurationValue)(nil)),
		ipType:       reflect.TypeOf((*IPValue)(nil)),
		ipNetType:    reflect.TypeOf((*IPNetValue)(nil)),
		ipmaskType:   reflect.TypeOf((*IPMaskValue)(nil)),
	}
	// kindAliases maps the kind of basic field types to the flag value pointer type that can alias them.
	kindAliases = map[reflect.Kind]reflect.Type{
		// unsigned integers
		reflect.Uint:   reflect.TypeOf((*UintValue)(nil)),
		reflect.Uint8:  reflect.TypeOf((*Uint8Value)(nil)),
		reflect.Uint16: reflect.TypeOf((*Uint16Value)(nil)),
		reflect.Uint32: reflect.TypeOf((*Uint32Value)(nil)),
		reflect.Uint64: reflect.TypeOf((*Uint64Value)(nil)),
		// signed integers
		reflect.Int:   reflect.TypeOf((*IntValue)(nil)),
		reflect.Int8:  reflect.TypeOf((*Int8Value)(nil)),
		reflect.Int16: reflect.TypeOf((*Int16Value)(nil)),
		reflect.Int32: reflect.TypeOf((*Int32Value)(nil)),
		reflect.Int64: reflect.TypeOf((*Int64Value)(nil)),
		// Misc
		reflect.String:  reflect.TypeOf((*StringValue)(nil)),
		reflect.Bool:    reflect.TypeOf((*BoolValue)(nil)),
		reflect.Float32: reflect.TypeOf((*Float32Value)(nil)),
		reflect.Float64: reflect.TypeOf((*Float64Value)(nil)),
	}
	// sliceTypeAliases maps exact slice element types to the flag value pointer type that can alias the slice.
	sliceTypeAliases = map[reflect.Type]reflect.Type{
		durationType: reflect.TypeOf((*DurationSliceValue)(nil)),
		ipType:       reflect.TypeOf((*IPSliceValue)(nil)),
	}
	// sliceKindAliases maps the kind of basic slice element types to the flag value pointer type that can alias the slice.
	sliceKindAliases = map[reflect.Kind]reflect.Type{
		reflect.Uint8:   reflect.TypeOf((*BytesHexFlag)(nil)),
		reflect.Uint16:  reflect.TypeOf((*Uint16SliceValue)(nil)),
		reflect.Uint32:  reflect.TypeOf((*Uint32SliceValue)(nil)),
		reflect.Uint64:  reflect.TypeOf((*Uint64SliceValue)(nil)),
		reflect.Uint:    reflect.TypeOf((*UintSliceValue)(nil)),
		reflect.Int8:    reflect.TypeOf((*Int8SliceValue)(nil)),
		reflect.Int16:   reflect.TypeOf((*Int16SliceValue)(nil)),
		reflect.Int32:   reflect.TypeOf((*Int32SliceValue)(nil)),
		reflect.Int64:   reflect.TypeOf((*Int64SliceValue)(nil)),
		reflect.Int:     reflect.TypeOf((*IntSliceValue)(nil)),
		reflect.Float32: reflect.TypeOf((*Float32SliceValue)(nil)),
		reflect.Float64: reflect.TypeOf((*Float64SliceValue)(nil)),
		reflect.String:  reflect.TypeOf((*StringSliceValue)(nil)),
		reflect.Bool:    reflect.TypeOf((*BoolSliceValue)(nil)),
	}
)

// alias binds a flag value of the given pointer type (e.g. *UintValue) to the address of val.
// This is only possible when the underlying types match, e.g. a uint field or a named uint type.
func alias(val reflect.Value, ptrTyp reflect.Type) (flag.Value, error) {
	addr := val.Addr()
	if !addr.Type().ConvertibleTo(ptrTyp) {
		return nil, fmt.Errorf("cannot bind %s to %s", val.Type(), ptrTyp.Elem())
	}
	return addr.Convert(ptrTyp).Interface().(flag.Value), nil
}

// FlagValue creates a flag.Value that reads and writes the given field value, which must be addressable.
// Types that implement flag.Value (or TypedValue) are used as-is, other supported types are adapted.
// Nil pointers are allocated.
func FlagValue(typ reflect.Type, val reflect.Value) (flag.Value, error) {
	if typ.Kind() == reflect.Ptr && val.IsNil() {
		if !val.CanSet() {
			return nil, fmt.Errorf("cannot allocate nil %s: not settable", typ)
		}
		val.Set(reflect.New(typ.Elem()))
	}
	// an interface field cannot be allocated: it must be pre-configured with a flag value
	if typ.Kind() == reflect.Interface && val.IsNil() {
		return nil, fmt.Errorf("nil interface %s must be set to a flag value before loading", typ)
	}

	// custom flag values take precedence over the built-in adapters
	if typ.Implements(typedFlagValueType) {
		return val.Interface().(TypedValue), nil
	} else if reflect.PtrTo(typ).Implements(typedFlagValueType) {
		return val.Addr().Interface().(TypedValue), nil
	} else if typ.Implements(flagValueType) {
		return val.Interface().(flag.Value), nil
	} else if reflect.PtrTo(typ).Implements(flagValueType) {
		return val.Addr().Interface().(flag.Value), nil
	}

	if ptrTyp, ok := typeAliases[typ]; ok {
		return alias(val, ptrTyp)
	}

	switch typ.Kind() {
	case reflect.Ptr:
		// recurse into the type
		return FlagValue(typ.Elem(), val.Elem())
	case reflect.Slice:
		elemTyp := typ.Elem()
		if ptrTyp, ok := sliceTypeAliases[elemTyp]; ok {
			return alias(val, ptrTyp)
		}
		if elemTyp.Kind() == reflect.Array {
			if elemTyp.Elem().Kind() != reflect.Uint8 {
				return nil, fmt.Errorf("unrecognized element type of array-element slice: %v", elemTyp.Elem())
			}
			return &fixedLenBytesSlice{Dest: val}, nil
		}
		ptrTyp, ok := sliceKindAliases[elemTyp.Kind()]
		if !ok {
			return nil, fmt.Errorf("unrecognized slice element type: %v", elemTyp)
		}
		if val.Addr().Type().ConvertibleTo(ptrTyp) {
			return alias(val, ptrTyp)
		}
		// A slice of a named element type (e.g. []MyID) cannot alias the typed slice values,
		// since the slice types differ. Read and write it with reflection instead.
		return &basicSliceValue{Dest: val}, nil
	case reflect.Array:
		if typ.Elem().Kind() != reflect.Uint8 {
			return nil, fmt.Errorf("unrecognized array element type: %v", typ.Elem())
		}
		expectedLen := val.Len()
		destSlice := val.Slice(0, expectedLen).Bytes()
		return &fixedLenBytes{
			Dest:           destSlice,
			ExpectedLength: uint64(expectedLen),
		}, nil
	default:
		ptrTyp, ok := kindAliases[typ.Kind()]
		if !ok {
			return nil, fmt.Errorf("unrecognized type: %v", typ)
		}
		return alias(val, ptrTyp)
	}
}
