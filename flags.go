package ask

import (
	"bytes"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type DurationValue time.Duration

func (d *DurationValue) Set(s string) error {
	v, err := time.ParseDuration(s)
	*d = DurationValue(v)
	return err
}

func (d *DurationValue) Type() string {
	return "duration"
}

func (d *DurationValue) String() string {
	return (*time.Duration)(d).String()
}

type IPValue net.IP

func (i *IPValue) String() string {
	return net.IP(*i).String()
}

func (i *IPValue) Set(s string) error {
	ip := net.ParseIP(s)
	if ip == nil {
		return fmt.Errorf("failed to parse IP: %q", s)
	}
	*i = IPValue(ip)
	return nil
}

func (i *IPValue) Type() string {
	return "ip"
}

type IPNetValue net.IPNet

func (ipnet IPNetValue) String() string {
	n := net.IPNet(ipnet)
	return n.String()
}

func (ipnet *IPNetValue) Set(s string) error {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		return err
	}
	*ipnet = IPNetValue(*n)
	return nil
}

func (*IPNetValue) Type() string {
	return "ipNet"
}

type IPMaskValue net.IPMask

func (i *IPMaskValue) String() string {
	return net.IPMask(*i).String()
}
func (i *IPMaskValue) Set(s string) error {
	ip := ParseIPv4Mask(s)
	if ip == nil {
		return fmt.Errorf("failed to parse IP mask: %q", s)
	}
	*i = IPMaskValue(ip)
	return nil
}

func (i *IPMaskValue) Type() string {
	return "ipMask"
}

// ParseIPv4Mask written in IP form (e.g. 255.255.255.0).
// This function should really belong to the net package.
func ParseIPv4Mask(s string) net.IPMask {
	mask := net.ParseIP(s)
	if mask == nil {
		if len(s) != 8 {
			return nil
		}
		// net.IPMask.String() actually outputs things like ffffff00
		// so write a horrible parser for that as well  :-(
		m := []int{}
		for i := 0; i < 4; i++ {
			b := "0x" + s[2*i:2*i+2]
			d, err := strconv.ParseInt(b, 0, 0)
			if err != nil {
				return nil
			}
			m = append(m, int(d))
		}
		s := fmt.Sprintf("%d.%d.%d.%d", m[0], m[1], m[2], m[3])
		mask = net.ParseIP(s)
		if mask == nil {
			return nil
		}
	}
	return net.IPv4Mask(mask[12], mask[13], mask[14], mask[15])
}

type UintValue uint

func (i *UintValue) Set(s string) error {
	v, err := strconv.ParseUint(s, 0, 64)
	*i = UintValue(v)
	return err
}

func (i *UintValue) Type() string {
	return "uint"
}

func (i *UintValue) String() string {
	return strconv.FormatUint(uint64(*i), 10)
}

type Uint8Value uint8

func (i *Uint8Value) Set(s string) error {
	v, err := strconv.ParseUint(s, 0, 8)
	*i = Uint8Value(v)
	return err
}

func (i *Uint8Value) Type() string {
	return "uint8"
}

func (i *Uint8Value) String() string {
	return strconv.FormatUint(uint64(*i), 10)
}

type Uint16Value uint16

func (i *Uint16Value) Set(s string) error {
	v, err := strconv.ParseUint(s, 0, 16)
	*i = Uint16Value(v)
	return err
}

func (i *Uint16Value) Type() string {
	return "uint16"
}

func (i *Uint16Value) String() string {
	return strconv.FormatUint(uint64(*i), 10)
}

type Uint32Value uint32

func (i *Uint32Value) Set(s string) error {
	v, err := strconv.ParseUint(s, 0, 32)
	*i = Uint32Value(v)
	return err
}

func (i *Uint32Value) Type() string {
	return "uint32"
}

func (i *Uint32Value) String() string {
	return strconv.FormatUint(uint64(*i), 10)
}

type Uint64Value uint64

func (i *Uint64Value) Set(s string) error {
	v, err := strconv.ParseUint(s, 0, 64)
	*i = Uint64Value(v)
	return err
}

func (i *Uint64Value) Type() string {
	return "uint64"
}

func (i *Uint64Value) String() string {
	return strconv.FormatUint(uint64(*i), 10)
}

type IntValue int

func (i *IntValue) Set(s string) error {
	v, err := strconv.ParseInt(s, 0, 64)
	*i = IntValue(v)
	return err
}

func (i *IntValue) Type() string {
	return "int"
}

func (i *IntValue) String() string {
	return strconv.Itoa(int(*i))
}

type Int8Value int8

func (i *Int8Value) Set(s string) error {
	v, err := strconv.ParseInt(s, 0, 8)
	*i = Int8Value(v)
	return err
}

func (i *Int8Value) Type() string {
	return "int8"
}

func (i *Int8Value) String() string {
	return strconv.FormatInt(int64(*i), 10)
}

type Int16Value int16

func (i *Int16Value) Set(s string) error {
	v, err := strconv.ParseInt(s, 0, 16)
	*i = Int16Value(v)
	return err
}

func (i *Int16Value) Type() string {
	return "int16"
}

func (i *Int16Value) String() string {
	return strconv.FormatInt(int64(*i), 10)
}

type Int32Value int32

func (i *Int32Value) Set(s string) error {
	v, err := strconv.ParseInt(s, 0, 32)
	*i = Int32Value(v)
	return err
}

func (i *Int32Value) Type() string {
	return "int32"
}

func (i *Int32Value) String() string {
	return strconv.FormatInt(int64(*i), 10)
}

type Int64Value int64

func (i *Int64Value) Set(s string) error {
	v, err := strconv.ParseInt(s, 0, 64)
	*i = Int64Value(v)
	return err
}

func (i *Int64Value) Type() string {
	return "int64"
}

func (i *Int64Value) String() string {
	return strconv.FormatInt(int64(*i), 10)
}

type StringValue string

func (s *StringValue) Set(val string) error {
	*s = StringValue(val)
	return nil
}
func (s *StringValue) Type() string {
	return "string"
}

func (s *StringValue) String() string {
	return string(*s)
}

type BoolValue bool

func (b *BoolValue) Set(s string) error {
	v, err := strconv.ParseBool(s)
	*b = BoolValue(v)
	return err
}

func (b *BoolValue) Type() string {
	return "bool"
}

func (b *BoolValue) String() string {
	return strconv.FormatBool(bool(*b))
}

type Float32Value float32

func (f *Float32Value) Set(s string) error {
	v, err := strconv.ParseFloat(s, 32)
	*f = Float32Value(v)
	return err
}

func (f *Float32Value) Type() string {
	return "float32"
}

func (f *Float32Value) String() string {
	return strconv.FormatFloat(float64(*f), 'g', -1, 32)
}

type Float64Value float64

func (f *Float64Value) Set(s string) error {
	v, err := strconv.ParseFloat(s, 64)
	*f = Float64Value(v)
	return err
}

func (f *Float64Value) Type() string {
	return "float64"
}

func (f *Float64Value) String() string {
	return strconv.FormatFloat(float64(*f), 'g', -1, 64)
}

type DurationSliceValue []time.Duration

func (s *DurationSliceValue) Set(val string) error {
	ss := strings.Split(val, ",")
	out := make([]time.Duration, len(ss))
	for i, d := range ss {
		var err error
		out[i], err = time.ParseDuration(d)
		if err != nil {
			return err
		}
	}
	*s = out
	return nil
}

func (s *DurationSliceValue) Type() string {
	return "durationSlice"
}

func (s *DurationSliceValue) String() string {
	out := make([]string, len(*s))
	for i, d := range *s {
		out[i] = d.String()
	}
	return strings.Join(out, ",")
}

type IPSliceValue []net.IP

func (s *IPSliceValue) Set(val string) error {
	ss := strings.Split(val, ",")
	out := make([]net.IP, len(ss))
	for i, d := range ss {
		out[i] = net.ParseIP(d)
		if out[i] == nil {
			return fmt.Errorf("invalid string being converted to IP address: %s", d)
		}
	}
	*s = out
	return nil
}

func (s *IPSliceValue) Type() string {
	return "ipSlice"
}

func (s *IPSliceValue) String() string {
	out := make([]string, len(*s))
	for i, d := range *s {
		out[i] = d.String()
	}
	return strings.Join(out, ",")
}

type Uint64SliceValue []uint64

func (s *Uint64SliceValue) Set(val string) error {
	ss := strings.Split(val, ",")
	out := make([]uint64, len(ss))
	for i, d := range ss {
		v, err := strconv.ParseUint(d, 0, 64)
		if err != nil {
			return err
		}
		out[i] = v
	}
	*s = out
	return nil
}

func (s *Uint64SliceValue) Type() string {
	return "uint64Slice"
}

func (s *Uint64SliceValue) String() string {
	out := make([]string, len(*s))
	for i, d := range *s {
		out[i] = fmt.Sprintf("%d", d)
	}
	return strings.Join(out, ",")
}

type Uint32SliceValue []uint32

func (s *Uint32SliceValue) Set(val string) error {
	ss := strings.Split(val, ",")
	out := make([]uint32, len(ss))
	for i, d := range ss {
		v, err := strconv.ParseUint(d, 0, 32)
		if err != nil {
			return err
		}
		out[i] = uint32(v)
	}
	*s = out
	return nil
}

func (s *Uint32SliceValue) Type() string {
	return "uint32Slice"
}

func (s *Uint32SliceValue) String() string {
	out := make([]string, len(*s))
	for i, d := range *s {
		out[i] = fmt.Sprintf("%d", d)
	}
	return strings.Join(out, ",")
}

type Uint16SliceValue []uint16

func (s *Uint16SliceValue) Set(val string) error {
	ss := strings.Split(val, ",")
	out := make([]uint16, len(ss))
	for i, d := range ss {
		v, err := strconv.ParseUint(d, 0, 16)
		if err != nil {
			return err
		}
		out[i] = uint16(v)
	}
	*s = out
	return nil
}

func (s *Uint16SliceValue) Type() string {
	return "uint16Slice"
}

func (s *Uint16SliceValue) String() string {
	out := make([]string, len(*s))
	for i, d := range *s {
		out[i] = fmt.Sprintf("%d", d)
	}
	return strings.Join(out, ",")
}

type UintSliceValue []uint

func (s *UintSliceValue) Set(val string) error {
	ss := strings.Split(val, ",")
	out := make([]uint, len(ss))
	for i, d := range ss {
		v, err := strconv.ParseUint(d, 0, 64)
		if err != nil {
			return err
		}
		out[i] = uint(v)
	}
	*s = out
	return nil
}

func (s *UintSliceValue) Type() string {
	return "uintSlice"
}

func (s *UintSliceValue) String() string {
	out := make([]string, len(*s))
	for i, d := range *s {
		out[i] = fmt.Sprintf("%d", d)
	}
	return strings.Join(out, ",")
}

type IntSliceValue []int

func (s *IntSliceValue) Set(val string) error {
	ss := strings.Split(val, ",")
	out := make([]int, len(ss))
	for i, d := range ss {
		v, err := strconv.ParseInt(d, 0, 64)
		if err != nil {
			return err
		}
		out[i] = int(v)
	}
	*s = out
	return nil
}

func (s *IntSliceValue) Type() string {
	return "intSlice"
}

func (s *IntSliceValue) String() string {
	out := make([]string, len(*s))
	for i, d := range *s {
		out[i] = fmt.Sprintf("%d", d)
	}
	return strings.Join(out, ",")
}

type StringSliceValue []string

func readAsCSV(val string) ([]string, error) {
	if val == "" {
		return []string{}, nil
	}
	stringReader := strings.NewReader(val)
	csvReader := csv.NewReader(stringReader)
	return csvReader.Read()
}

func writeAsCSV(vals []string) (string, error) {
	b := &bytes.Buffer{}
	w := csv.NewWriter(b)
	err := w.Write(vals)
	if err != nil {
		return "", err
	}
	w.Flush()
	return strings.TrimSuffix(b.String(), "\n"), nil
}

func (s *StringSliceValue) Set(val string) error {
	v, err := readAsCSV(val)
	if err != nil {
		return err
	}
	*s = v
	return nil
}

func (s *StringSliceValue) Type() string {
	return "stringSlice"
}

func (s *StringSliceValue) String() string {
	str, _ := writeAsCSV(*s)
	return str
}

type BoolSliceValue []bool

func (s *BoolSliceValue) Set(val string) error {
	ss := strings.Split(val, ",")
	out := make([]bool, len(ss))
	for i, d := range ss {
		v, err := strconv.ParseBool(d)
		if err != nil {
			return err
		}
		out[i] = v
	}
	*s = out
	return nil
}

func (s *BoolSliceValue) Type() string {
	return "boolSlice"
}

func (s *BoolSliceValue) String() string {
	boolStrSlice := make([]string, len(*s))
	for i, b := range *s {
		boolStrSlice[i] = strconv.FormatBool(b)
	}

	return strings.Join(boolStrSlice, ",")
}

// BytesHex exposes bytes as a flag, hex-encoded,
// optional whitespace padding, case insensitive, and optional 0x prefix.
type BytesHexFlag []byte

func (f BytesHexFlag) String() string {
	return hex.EncodeToString(f)
}

func (f *BytesHexFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	value = strings.ToLower(value)
	if strings.HasPrefix(value, "0x") {
		value = value[2:]
	}
	b, err := hex.DecodeString(value)
	if err != nil {
		return err
	}
	*f = b
	return nil
}

func (f *BytesHexFlag) Type() string {
	return "bytes"
}

// fixedLenBytes exposes fixed-length bytes as a flag, hex-encoded,
// optional whitespace padding, case insensitive, and optional 0x prefix.
type fixedLenBytes struct {
	Dest           []byte
	ExpectedLength uint64
}

func (f fixedLenBytes) String() string {
	return hex.EncodeToString(f.Dest)
}

func (f *fixedLenBytes) Set(value string) error {
	value = strings.TrimSpace(value)
	value = strings.ToLower(value)
	if strings.HasPrefix(value, "0x") {
		value = value[2:]
	}
	b, err := hex.DecodeString(value)
	if err != nil {
		return err
	}
	if uint64(len(b)) != f.ExpectedLength {
		return fmt.Errorf("byte length does not match fixed-length of %d bytes: "+
			"parsed %d bytes", f.ExpectedLength, len(b))
	}
	copy(f.Dest, b)
	return nil
}

func (f *fixedLenBytes) Type() string {
	return fmt.Sprintf("bytes%d", f.ExpectedLength)
}

// fixedLenBytesSlice exposes a slice of fixed-length bytes elements as a flag,
// optional whitespace/padding, comma-separated.
// Each element is hex-encoded, case insensitive, and optional 0x prefix.
type fixedLenBytesSlice struct {
	Dest reflect.Value
}

func (f fixedLenBytesSlice) String() string {
	var buf strings.Builder
	length := f.Dest.Len()
	elemLen := f.Dest.Type().Elem().Len()
	for i := 0; i < length; i++ {
		if i > 0 {
			buf.WriteString(",")
		}
		item := f.Dest.Index(i)
		itemBytes := item.Slice(0, elemLen).Bytes()
		buf.WriteString(hex.EncodeToString(itemBytes))
	}
	return buf.String()
}

func (f *fixedLenBytesSlice) Set(value string) error {
	value = strings.TrimSpace(value)
	value = strings.ToLower(value)
	var elems []string
	if len(value) > 0 {
		elems = strings.Split(value, ",")
	}
	dest := reflect.MakeSlice(f.Dest.Type(), len(elems), len(elems))
	elemTyp := f.Dest.Type().Elem()
	elemLen := elemTyp.Len()
	for i, el := range elems {
		el = strings.TrimSpace(el)
		if strings.HasPrefix(el, "0x") {
			el = el[2:]
		}
		b, err := hex.DecodeString(el)
		if err != nil {
			return err
		}
		if len(b) != elemLen {
			return fmt.Errorf("byte length of element %d does not match fixed-length of %d bytes: "+
				"parsed %d bytes", i, elemLen, len(b))
		}
		destElem := dest.Index(i)
		destElemBytes := destElem.Slice(0, elemLen).Bytes()
		copy(destElemBytes, b)
	}
	f.Dest.Set(dest)
	return nil
}

func (f *fixedLenBytesSlice) Type() string {
	elemLen := f.Dest.Type().Elem().Len()
	return fmt.Sprintf("[]bytes%d", elemLen)
}
