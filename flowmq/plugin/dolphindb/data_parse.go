package dolphindb

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dolphindb/api-go/v3/model"
)

var dataTypeByteMap = map[string]model.DataTypeByte{
	"void":          model.DtVoid,
	"bool":          model.DtBool,
	"char":          model.DtChar,
	"short":         model.DtShort,
	"int":           model.DtInt,
	"long":          model.DtLong,
	"date":          model.DtDate,
	"month":         model.DtMonth,
	"time":          model.DtTime,
	"minute":        model.DtMinute,
	"second":        model.DtSecond,
	"datetime":      model.DtDatetime,
	"timestamp":     model.DtTimestamp,
	"nanotime":      model.DtNanoTime,
	"nanotimestamp": model.DtNanoTimestamp,
	"float":         model.DtFloat,
	"double":        model.DtDouble,
	"symbol":        model.DtSymbol,
	"string":        model.DtString,
	"uuid":          model.DtUUID,
	"function":      model.DtFunction,
	"handle":        model.DtHandle,
	"code":          model.DtCode,
	"datasource":    model.DtDatasource,
	"resource":      model.DtResource,
	"any":           model.DtAny,
	"compress":      model.DtCompress,
	"dictionary":    model.DtDictionary,
	"datehour":      model.DtDateHour,
	"dateminute":    model.DtDateMinute,
	"ipaddr":        model.DtIP,
	"int128":        model.DtInt128,
	"blob":          model.DtBlob,
	"complex":       model.DtComplex,
	"point":         model.DtPoint,
	"duration":      model.DtDuration,
	"decimal32":     model.DtDecimal32,
	"decimal64":     model.DtDecimal64,
	"decimal128":    model.DtDecimal128,
	"object":        model.DtObject,
}

func getDataTypeFromString(typeStr string) (model.DataTypeByte, error) {
	dataType, ok := dataTypeByteMap[typeStr]
	if !ok {
		return 0, fmt.Errorf("unsupported data type: %s", typeStr)
	}
	return dataType, nil
}

func renderData(datatype model.DataTypeByte, data interface{}) (interface{}, error) {
	switch datatype {
	case model.DtBool:
		if _, ok := data.(bool); ok {
			return data, nil
		}
		//int value 1 is treated as true; other int is treated as false
		if intVal, ok := data.(int); ok {
			return intVal == 1, nil
		}
		//string value true/TRUE is treated as true; other string is treated as false
		if strVal, ok := data.(string); ok {
			return strings.ToLower(strVal) == "true", nil
		}
		//other values are treated as false
		return false, nil
	case model.DtBlob:
		if _, ok := data.([]byte); ok {
			return data, nil
		}
		return nil, errors.New("failed to parse to []byte")
	case model.DtChar, model.DtCompress:
		if _, ok := data.(byte); ok {
			return data, nil
		}
		return nil, errors.New("failed to parse to byte")
	case model.DtComplex, model.DtPoint:
		if _, ok := data.([2]float64); ok {
			return data, nil
		}
		return nil, errors.New("failed to parse to [2]float64")
	case model.DtDate, model.DtDateHour, model.DtDatetime, model.DtDateMinute,
		model.DtMinute, model.DtMonth, model.DtNanoTime, model.DtSecond, model.DtNanoTimestamp,
		model.DtTime, model.DtTimestamp:
		if _, ok := data.(time.Time); ok {
			return data, nil
		}
		//RFC3339 time layout is supported
		if strVal, ok := data.(string); ok {
			if t, err := time.Parse(time.RFC3339Nano, strVal); err == nil {
				return t, nil
			}
			if t, err := time.Parse(time.RFC3339, strVal); err == nil {
				return t, nil
			}
		}
		return nil, errors.New("failed to parse to time.Time")
	case model.DtDouble:
		if fVal, ok := data.(float64); ok {
			return float64(fVal), nil
		}
		return nil, errors.New("failed to parse to float64")
	case model.DtFloat:
		if fVal, ok := data.(float64); ok {
			return float32(fVal), nil
		}
		return nil, errors.New("failed to parse to float32")
	case model.DtString, model.DtDuration, model.DtInt128, model.DtIP, model.DtUUID,
		model.DtCode, model.DtFunction, model.DtHandle, model.DtSymbol:
		if _, ok := data.(string); ok {
			return data, nil
		}
		return nil, errors.New("failed to parse to string")
	case model.DtInt:
		if intVal, ok := data.(int64); ok {
			return int32(intVal), nil
		}
		return nil, errors.New("failed to parse to int32")
	case model.DtLong:
		if intVal, ok := data.(int64); ok {
			return int64(intVal), nil
		}
		return nil, errors.New("failed to parse to int64")
	case model.DtShort:
		if intVal, ok := data.(int64); ok {
			return int16(intVal), nil
		}
		return nil, errors.New("failed to parse to int16")

	default:
		return data, nil
	}
}

func getIntFromDataType(dt model.DataType) (int, error) {
	scalar, ok := dt.Value().(*model.Scalar)
	if !ok {
		return -1, errors.New("failed to get int value")
	}
	return getIntFromScalar(scalar)
}

func getIntFromScalar(scalar *model.Scalar) (int, error) {
	switch v := scalar.DataType.Value().(type) {
	case int32:
		return int(v), nil
	case int64:
		return int(v), nil
	case int:
		return v, nil
	default:
		return -1, errors.New("failed to get int value")
	}
}

func getStringFromDataType(dt model.DataType) (string, error) {
	if scalar, ok := dt.Value().(*model.Scalar); ok {
		switch v := scalar.DataType.Value().(type) {
		case string:
			return v, nil
		default:
			return "", errors.New("failed to get string value")
		}
	}
	if pair, ok := dt.Value().(*model.Pair); ok {
		if pair.Vector == nil {
			return "", nil
		}
		return getStringFromVector(pair.Vector)
	}
	if vector, ok := dt.Value().(*model.Vector); ok {
		return getStringFromVector(vector)
	}
	return "", errors.New("failed to get string value")
}

func getStringFromVector(vector *model.Vector) (string, error) {
	result := []string{}
	len := vector.Rows()
	for i := 0; i < len; i++ {
		dt := vector.Get(i)
		switch v := dt.Value().(type) {
		case string:
			result = append(result, v)
		default:
			return "", errors.New("failed to get string value")
		}
	}
	return strings.Join(result, ". "), nil
}
