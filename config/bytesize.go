package config

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type ByteSize int64

const (
	KiB ByteSize = 1 << 10
	MiB ByteSize = 1 << 20
	GiB ByteSize = 1 << 30
	TiB ByteSize = 1 << 40
)

type byteSizeUnit struct {
	suffix string
	factor int64
}

var byteSizeParseUnits = []byteSizeUnit{
	{"kib", int64(KiB)},
	{"mib", int64(MiB)},
	{"gib", int64(GiB)},
	{"tib", int64(TiB)},
	{"kb", 1000},
	{"mb", 1000 * 1000},
	{"gb", 1000 * 1000 * 1000},
	{"tb", 1000 * 1000 * 1000 * 1000},
	{"b", 1},
}

var byteSizeFormatUnits = []struct {
	suffix string
	factor ByteSize
}{
	{"TiB", TiB},
	{"GiB", GiB},
	{"MiB", MiB},
	{"KiB", KiB},
}

func ParseByteSize(text string) (ByteSize, error) {
	trimmed := strings.TrimSpace(text)
	if "" == trimmed {
		return 0, fmt.Errorf("byte size must not be empty")
	}
	lowered := strings.ToLower(trimmed)
	number := trimmed
	factor := int64(1)
	for _, unit := range byteSizeParseUnits {
		if strings.HasSuffix(lowered, unit.suffix) {
			number = strings.TrimSpace(trimmed[:len(trimmed)-len(unit.suffix)])
			factor = unit.factor
			break
		}
	}
	if "" == number {
		return 0, fmt.Errorf("byte size %q has no number", text)
	}
	if whole, err := strconv.ParseInt(number, 10, 64); nil == err {
		if whole > math.MaxInt64/factor || whole < math.MinInt64/factor {
			return 0, fmt.Errorf("byte size %q is out of range", text)
		}
		return ByteSize(whole * factor), nil
	}
	fraction, err := strconv.ParseFloat(number, 64)
	if nil != err {
		return 0, fmt.Errorf("byte size %q is not a number with an optional unit such as 256MiB", text)
	}
	product := math.Round(fraction * float64(factor))
	if math.IsNaN(product) || math.IsInf(product, 0) || product >= math.MaxInt64 || product <= math.MinInt64 {
		return 0, fmt.Errorf("byte size %q is out of range", text)
	}
	return ByteSize(product), nil
}

func (b ByteSize) String() string {
	if 0 == b {
		return "0"
	}
	for _, unit := range byteSizeFormatUnits {
		if 0 == b%unit.factor {
			return strconv.FormatInt(int64(b/unit.factor), 10) + unit.suffix
		}
	}
	return strconv.FormatInt(int64(b), 10) + "B"
}

func (b ByteSize) Int64() int64 {
	return int64(b)
}

func (b ByteSize) MarshalText() ([]byte, error) {
	return []byte(b.String()), nil
}

func (b *ByteSize) UnmarshalText(text []byte) error {
	parsed, err := ParseByteSize(string(text))
	if nil != err {
		return err
	}
	*b = parsed
	return nil
}
