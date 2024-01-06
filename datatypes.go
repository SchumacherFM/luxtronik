package luxtronik

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/spf13/cast"
)

const (
	classEnergy      = "energy"
	classTemperature = "temperature"
	classNone        = "none"
	classSelection   = "selection"
	classFrequency   = "frequency"
	classCount       = "count"
	classDuration    = "duration"
	classTime        = "time"
	classBool        = "bool"
)

var ErrWritingNotAllowed = errors.New("writing to heat pump not allowed or not possible")

type DataTypeMap map[int]*Base

func (pm DataTypeMap) IterateSorted(cb func(int, *Base)) {
	keys := lo.Keys(pm)
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})
	for _, key := range keys {
		cb(key, pm[key])
	}
}

func (pm DataTypeMap) SetRawValues(data []uint32) error {
	if dl, pml := len(data), len(pm); dl != pml {
		return fmt.Errorf("DataTypeMap.SetRawValues length of data:%d not equal to length of DataTypeMap:%d", dl, pml)
	}

	for idx, raw := range data {
		pm[idx].SetRaw(raw)
	}

	return nil
}

func (pm DataTypeMap) GetVersion() string {
	var buf strings.Builder
	for i := 81; i <= 87; i++ {
		buf.WriteString(pm[i].FromHeatPump().(string))
	}
	return buf.String()
}

type Base struct {
	customFromHP  func(uint32) any
	customToHP    func(any) (uint32, error)
	codes         []string
	returnType    reflect.Kind
	name          string
	class         string
	luxtronikName string
	unit          string
	rawValue      uint32
	prevRawValue  uint32
	factor        float32
	writeable     bool
}

func (b *Base) String() string {
	return fmt.Sprintf("class:%q name:%q unit:%q writeable:%t",
		b.class,
		b.luxtronikName,
		b.unit,
		b.writeable,
	)
}

func (b *Base) Name() string {
	return b.luxtronikName
}

func (b *Base) Unit() string {
	return b.unit
}

func (b *Base) SetRaw(val uint32) {
	b.prevRawValue = b.rawValue
	b.rawValue = val
}

func (b *Base) HasChanges() bool {
	return b.prevRawValue != b.rawValue
}

func (b *Base) FromHeatPump() any {
	if b.codes != nil {
		if b.rawValue > uint32(len(b.codes)) {
			return fmt.Sprintf("unknown code: %d", b.rawValue)
		}

		return b.codes[b.rawValue]
	}

	if b.customFromHP != nil {
		return b.customFromHP(b.rawValue)
	}
	if b.class == classDuration {
		if b.name == "seconds" {
			return time.Duration(b.rawValue) * time.Second
		}
	}

	switch b.returnType {
	case reflect.Uint32:
		if b.factor != 0 {
			return uint32(float32(b.rawValue) * b.factor)
		}
		return b.rawValue

	case reflect.Float32:

		if b.factor != 0 {
			return roundFloat(float64(b.rawValue)*float64(b.factor), 3)
		}
		return float32(b.rawValue)

	default:
		return b.rawValue
	}
}

func roundFloat(val float64, precision uint) float32 {
	ratio := math.Pow(10, float64(precision))
	return float32(math.Round(val*ratio) / ratio)
}

func (b *Base) ToHeatPump(val any) (uint32, error) {
	if !b.writeable {
		return 0, fmt.Errorf("ToHeatPump can't write non-writeable value: %v", val)
	}

	if b.codes != nil {
		vals := cast.ToString(val)
		for idx, code := range b.codes {
			if code == vals {
				return uint32(idx), nil
			}
		}
		return 0, fmt.Errorf("ToHeatPump can't find value: %q in list of codes", vals)
	}
	if b.customToHP != nil {
		return b.customToHP(b.rawValue)
	}

	switch b.returnType {
	case reflect.Uint32:
		if b.factor != 0 {
			return uint32(cast.ToFloat32(val) / b.factor), nil
		}
		return b.rawValue, nil

	case reflect.Float32:
		if b.factor != 0 {
			return uint32(cast.ToFloat32(val) / b.factor), nil
		}
		return cast.ToUint32(val), nil

	default:
		return b.rawValue, nil
	}
}

func NewEnergy(name string) *Base {
	return &Base{
		returnType:    reflect.Float32,
		name:          "energy",
		class:         classEnergy,
		luxtronikName: name,
		unit:          "kWh",
		factor:        0.1,
	}
}

func NewCelsius(name string, writeable bool) *Base {
	return &Base{
		returnType:    reflect.Float32,
		name:          "celsius",
		class:         classTemperature,
		luxtronikName: name,
		unit:          "°C",
		writeable:     writeable,
		factor:        0.1,
	}
}

func NewKelvin(name string, writeable bool) *Base {
	return &Base{
		returnType:    reflect.Float32,
		name:          "kelvin",
		class:         classTemperature,
		luxtronikName: name,
		unit:          "K",
		writeable:     writeable,
		factor:        0.1,
	}
}

func NewVoltage(name string) *Base {
	return &Base{
		returnType:    reflect.Float32,
		name:          "Voltage",
		class:         "voltage",
		luxtronikName: name,
		unit:          "V",
		factor:        0.1,
	}
}

func NewFlow(name string) *Base {
	return &Base{
		returnType:    reflect.Float32,
		name:          "Flow",
		class:         "flow",
		luxtronikName: name,
		unit:          "l/h",
	}
}

func NewPressure(name string) *Base {
	return &Base{
		returnType:    reflect.Float32,
		name:          "Pressure",
		class:         "pressure",
		factor:        0.01,
		luxtronikName: name,
		unit:          "bar",
	}
}

func NewUnknown(name string) *Base {
	return &Base{
		returnType:    reflect.Uint32,
		name:          classNone,
		class:         "none",
		luxtronikName: name,
		writeable:     false,
	}
}

func NewHeatingMode(name string, writeable bool) *Base {
	return &Base{
		returnType:    reflect.String,
		name:          "HeatingMode",
		luxtronikName: name,
		class:         classSelection,
		writeable:     writeable,
		codes: []string{
			0: "Automatic",
			1: "Second heatsource",
			2: "Party",
			3: "Holidays",
			4: "Off",
		},
	}
}

func NewHotWaterMode(name string, writeable bool) *Base {
	b := NewHeatingMode(name, writeable)
	b.name = "HotWaterMode"
	return b
}

func NewPoolMode(name string, writeable bool) *Base {
	b := NewHeatingMode(name, writeable)
	b.codes[1] = ""
	b.name = "PoolMode"
	return b
}

func NewAccessLevel(name string, writeable bool) *Base {
	return &Base{
		returnType:    reflect.String,
		name:          "AccessLevel",
		luxtronikName: name,
		class:         "selection",
		writeable:     writeable,
		codes: []string{
			0: "user",
			1: "after sales service",
			2: "manufacturer",
			3: "installer",
		},
	}
}

func NewMixedCircuitMode(name string, writeable bool) *Base {
	return &Base{
		returnType:    reflect.String,
		name:          "MixedCircuitMode",
		luxtronikName: name,
		class:         "selection",
		writeable:     writeable,
		codes: []string{
			0: "Automatic", 2: "Party", 3: "Holidays", 4: "Off",
		},
	}
}

func NewFrequency(name string) *Base {
	return &Base{
		returnType:    reflect.Float32,
		name:          "Frequency",
		class:         "frequency",
		luxtronikName: name,
		unit:          "Hz",
	}
}

func NewIcon(name string) *Base {
	return &Base{
		returnType:    reflect.Uint32,
		name:          "Icon",
		class:         "icon",
		luxtronikName: name,
	}
}

func NewPercent2(name string) *Base {
	return &Base{
		returnType:    reflect.Uint32,
		name:          "Percent2",
		class:         "percent",
		luxtronikName: name,
		unit:          "%",
	}
}

func NewSpeed(name string) *Base {
	return &Base{
		returnType:    reflect.Uint32,
		name:          "Speed",
		class:         "speed",
		luxtronikName: name,
		unit:          "rpm",
	}
}

func NewPower(name string) *Base {
	return &Base{
		returnType:    reflect.Uint32,
		name:          "Power",
		class:         "power",
		luxtronikName: name,
		unit:          "W",
	}
}

func NewCount(name string) *Base {
	return &Base{
		returnType:    reflect.Uint32,
		name:          "Count",
		class:         classCount,
		luxtronikName: name,
	}
}

func NewLevel(name string) *Base {
	return &Base{
		returnType:    reflect.Uint32,
		name:          "Level",
		class:         classCount,
		luxtronikName: name,
	}
}

func NewErrorcode(name string) *Base {
	return &Base{
		returnType:    reflect.Uint32,
		name:          "Errorcode",
		class:         "value",
		luxtronikName: name,
	}
}

func NewSeconds(name string) *Base {
	return &Base{
		returnType:    reflect.Int64,
		name:          "seconds",
		class:         classDuration,
		luxtronikName: name,
		unit:          "s",
	}
}

func NewHours(name string, writeable bool) *Base {
	return &Base{
		returnType:    reflect.Int64,
		name:          "hours",
		class:         classDuration,
		luxtronikName: name,
		unit:          "h",
		writeable:     writeable,
		factor:        0.1,
	}
}

func NewHours2(name string, writeable bool) *Base {
	return &Base{
		customFromHP: func(val uint32) any {
			return 1 + val/2
		},
		customToHP: func(val any) (uint32, error) {
			return (cast.ToUint32(val) - 1) * 2, nil
		},
		returnType:    reflect.Int64,
		name:          "hours2",
		class:         classDuration,
		luxtronikName: name,
		unit:          "h",
		writeable:     writeable,
	}
}

func NewMinutes(name string, writeable bool) *Base {
	return &Base{
		returnType:    reflect.Int64,
		name:          "minutes",
		class:         classDuration,
		luxtronikName: name,
		unit:          "min",
		writeable:     writeable,
	}
}

func NewTime(name string) *Base {
	return &Base{
		customFromHP: func(val uint32) any {
			if val < 1 {
				return ""
			}
			return time.Unix(int64(val), 0).Format("2006-01-02 15:04:05")
		},
		customToHP: func(val any) (uint32, error) {
			t, err := time.Parse("2006-01-02 15:04:05", cast.ToString(val))
			return uint32(t.Unix()), err
		},
		returnType:    reflect.String,
		name:          "time",
		class:         classTime,
		luxtronikName: name,
		unit:          "ts",
	}
}

func NewMajorMinorVersion(name string) *Base {
	return &Base{
		customFromHP: func(val uint32) any {
			if val > 0 {
				major := val / 100
				minor := val % 100
				return fmt.Sprintf("%d.%d", major, minor)

			}
			return "0"
		},
		customToHP: func(val any) (uint32, error) {
			return 0, ErrWritingNotAllowed
		},
		returnType:    reflect.String,
		name:          "MajorMinorVersion",
		class:         "version",
		luxtronikName: name,
		unit:          "",
	}
}

func NewCoolingMode(name string, writeable bool) *Base {
	return &Base{
		returnType:    reflect.String,
		name:          "CoolingMode",
		luxtronikName: name,
		class:         "selection",
		writeable:     writeable,
		codes: []string{
			0: "Off", 1: "Automatic",
		},
	}
}

func NewSolarMode(name string, writeable bool) *Base {
	m := NewCoolingMode(name, writeable)
	m.name = "SolarMode"
	return m
}

func NewVentilationMode(name string, writeable bool) *Base {
	return &Base{
		returnType:    reflect.String,
		name:          "VentilationMode",
		luxtronikName: name,
		class:         "selection",
		writeable:     writeable,
		codes: []string{
			0: "Automatic", 1: "Party", 2: "Holidays", 3: "Off",
		},
	}
}

func NewBool(name string, writeable bool) *Base {
	return &Base{
		customFromHP: func(val uint32) any {
			return val == 1
		},
		customToHP: func(val any) (uint32, error) {
			if cast.ToBool(val) {
				return 1, nil
			}
			return 0, nil
		},
		returnType:    reflect.Bool,
		name:          "Bool",
		class:         "boolean",
		luxtronikName: name,
		writeable:     writeable,
	}
}

func NewIPV4Address(name string) *Base {
	return &Base{
		customFromHP: func(val uint32) any {
			var b [SocketReadSizeInteger]byte
			binary.BigEndian.PutUint32(b[:], val)
			a := netip.AddrFrom4(b)
			return a.String()
		},
		customToHP: func(a any) (uint32, error) {
			panic("todo implement")
		},
		returnType:    reflect.String,
		name:          "IPAddress",
		class:         "string",
		luxtronikName: name,
	}
}

func NewHeatpumpCode(name string) *Base {
	return &Base{
		returnType:    reflect.String,
		name:          "HeatpumpCode",
		luxtronikName: name,
		class:         classSelection,
		codes: []string{
			// please keep the index as it is directly assigned to the code in
			// the heatpump
			0:  "ERC",
			1:  "SW1",
			2:  "SW2",
			3:  "WW1",
			4:  "WW2",
			5:  "L1I",
			6:  "L2I",
			7:  "L1A",
			8:  "L2A",
			9:  "KSW",
			10: "KLW",
			11: "SWC",
			12: "LWC",
			13: "L2G",
			14: "WZS",
			15: "L1I407",
			16: "L2I407",
			17: "L1A407",
			18: "L2A407",
			19: "L2G407",
			20: "LWC407",
			21: "L1AREV",
			22: "L2AREV",
			23: "WWC1",
			24: "WWC2",
			25: "L2G404",
			26: "WZW",
			27: "L1S",
			28: "L1H",
			29: "L2H",
			30: "WZWD",
			31: "ERC",
			40: "WWB_20",
			41: "LD5",
			42: "LD7",
			43: "SW 37_45",
			44: "SW 58_69",
			45: "SW 29_56",
			46: "LD5 (230V)",
			47: "LD7 (230 V)",
			48: "LD9",
			49: "LD5 REV",
			50: "LD7 REV",
			51: "LD5 REV 230V",
			52: "LD7 REV 230V",
			53: "LD9 REV 230V",
			54: "SW 291",
			55: "LW SEC",
			56: "HMD 2",
			57: "MSW 4",
			58: "MSW 6",
			59: "MSW 8",
			60: "MSW 10",
			61: "MSW 12",
			62: "MSW 14",
			63: "MSW 17",
			64: "MSW 19",
			65: "MSW 23",
			66: "MSW 26",
			67: "MSW 30",
			68: "MSW 4S",
			69: "MSW 6S",
			70: "MSW 8S",
			71: "MSW 10S",
			72: "MSW 13S",
			73: "MSW 16S",
			74: "MSW2-6S",
			75: "MSW4-16",
			76: "TODO unknown 76",
			77: "TODO unknown 77",
			78: "TODO unknown 78",
			79: "TODO unknown 79",
			80: "TODO unknown 80",
			81: "TODO unknown 81",
			82: "TODO unknown 82",
		},
	}
}

func NewBivalenceLevel(name string) *Base {
	return &Base{
		returnType:    reflect.String,
		name:          "BivalenceLevel",
		luxtronikName: name,
		class:         "selection",
		codes: []string{
			1: "one compressor allowed to run",
			2: "two compressors allowed to run",
			3: "additional heat generator allowed to run",
		},
	}
}

func NewOperationMode(name string) *Base {
	return &Base{
		returnType:    reflect.String,
		name:          "OperationMode",
		luxtronikName: name,
		class:         "selection",
		codes: []string{
			0: "heating",
			1: "hot water",
			2: "swimming pool/solar",
			3: "evu",
			4: "defrost",
			5: "no request",
			6: "heating external source",
			7: "cooling",
		},
	}
}

func NewCharacter(name string) *Base {
	return &Base{
		returnType:    reflect.String,
		name:          "Character",
		luxtronikName: name,
		class:         "string",
		customFromHP: func(u uint32) any {
			if 0 == u {
				return ""
			}
			if int(u) < len(charTable) {
				// we can later refactor it to use string(u) but afaik that
				// conversion might lead to bugs as not documented in Go.
				return charTable[u]
			}
			return fmt.Sprintf("char %d:%x not found", u, u)
		},
	}
}

var charTable = [127]string{
	'#':  "#",
	'!':  "!",
	'$':  "$",
	'%':  "%",
	'&':  "&",
	'\'': "'",
	'*':  "*",
	'+':  "+",
	'-':  "-",
	'.':  ".",
	'0':  "0",
	'1':  "1",
	'2':  "2",
	'3':  "3",
	'4':  "4",
	'5':  "5",
	'6':  "6",
	'7':  "7",
	'8':  "8",
	'9':  "9",
	'A':  "A",
	'B':  "B",
	'C':  "C",
	'D':  "D",
	'E':  "E",
	'F':  "F",
	'G':  "G",
	'H':  "H",
	'I':  "I",
	'J':  "J",
	'K':  "K",
	'L':  "L",
	'M':  "M",
	'N':  "N",
	'O':  "O",
	'P':  "P",
	'Q':  "Q",
	'R':  "R",
	'S':  "S",
	'T':  "T",
	'U':  "U",
	'W':  "W",
	'V':  "V",
	'X':  "X",
	'Y':  "Y",
	'Z':  "Z",
	'^':  "^",
	'_':  "_",
	'`':  "`",
	'a':  "a",
	'b':  "b",
	'c':  "c",
	'd':  "d",
	'e':  "e",
	'f':  "f",
	'g':  "g",
	'h':  "h",
	'i':  "i",
	'j':  "j",
	'k':  "k",
	'l':  "l",
	'm':  "m",
	'n':  "n",
	'o':  "o",
	'p':  "p",
	'q':  "q",
	'r':  "r",
	's':  "s",
	't':  "t",
	'u':  "u",
	'v':  "v",
	'w':  "w",
	'x':  "x",
	'y':  "y",
	'z':  "z",
	'|':  "|",
	'~':  "~",
}

func NewSwitchoffFile(name string) *Base {
	return &Base{
		returnType:    reflect.String,
		name:          "SwitchoffFile",
		luxtronikName: name,
		class:         "selection",
		codes: []string{
			1:  "heatpump error",
			2:  "system error",
			3:  "evu lock",
			4:  "operation mode second heat generator",
			5:  "air defrost",
			6:  "maximal usage temperature",
			7:  "minimal usage temperature",
			8:  "lower usage limit",
			9:  "no request",
			11: "flow rate",
			19: "PV max",
		},
	}
}

func NewMainMenuStatusLine1(name string) *Base {
	return &Base{
		returnType:    reflect.String,
		name:          "MainMenuStatusLine1",
		luxtronikName: name,
		class:         "selection",
		codes: []string{
			0: "heatpump running",
			1: "heatpump idle",
			2: "heatpump coming",
			3: "errorcode slot 0",
			4: "defrost",
			5: "waiting on LIN connection",
			6: "compressor heating up",
			7: "pump forerun",
		},
	}
}

func NewMainMenuStatusLine2(name string) *Base {
	return &Base{
		returnType:    reflect.String,
		name:          "MainMenuStatusLine2",
		luxtronikName: name,
		class:         "selection",
		codes: []string{
			0: "since", 1: "in",
		},
	}
}

func NewMainMenuStatusLine3(name string) *Base {
	return &Base{
		returnType:    reflect.String,
		name:          "MainMenuStatusLine3",
		luxtronikName: name,
		class:         "selection",
		codes: []string{
			0:  "heating",
			1:  "no request",
			2:  "grid switch on delay",
			3:  "cycle lock",
			4:  "lock time",
			5:  "domestic water",
			6:  "info bake out program",
			7:  "defrost",
			8:  "pump forerun",
			9:  "thermal desinfection",
			10: "cooling",
			12: "swimming pool/solar",
			13: "heating external energy source",
			14: "domestic water external energy source",
			16: "flow monitoring",
			17: "second heat generator 1 active",
		},
	}
}

func NewSecOperationMode(name string) *Base {
	return &Base{
		returnType:    reflect.String,
		name:          "SecOperationMode",
		luxtronikName: name,
		class:         "selection",
		codes: []string{
			0:  "off",
			1:  "cooling",
			2:  "heating",
			3:  "fault",
			4:  "transition",
			5:  "defrost",
			6:  "waiting",
			7:  "waiting",
			8:  "transition",
			9:  "stop",
			10: "manual",
			11: "simulation start",
			12: "evu lock",
		},
	}
}
