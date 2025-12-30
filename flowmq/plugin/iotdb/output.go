package iotdb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/apache/iotdb-client-go/v2/client"
	"github.com/warpstreamlabs/bento/public/service"
)

const (
	// Connection Fields
	fieldNodeUrls        = "urls"
	fieldDBUsername      = "username"
	fieldDBPassword      = "password"
	fieldConnectTimeout  = "connect_timeout"
	fieldConnectRetryMax = "connect_retry_max"
	fieldPoolSize        = "pool_size"

	fieldDeviceId           = "device_id"
	fieldMeasurement        = "measurement"
	fieldDataType           = "data_type"
	fieldTimestampPrecision = "timestamp_precision"

	metaDeviceId         = "device_id"
	metaMeasurement      = "measurement"
	metaDataType         = "data_type"
	metaTimestamp        = "timestamp"
	metaValue            = "value"
	metaMeasurementArray = "measurement_array"
	metaDataTypeArray    = "data_type_array"
	metaValueArray       = "value_array"
)

type iotdbWriter struct {
	log *service.Logger

	poolConfig       *client.PoolConfig
	sessionPool      *client.SessionPool
	connectTimeoutMs int
	maxPoolSize      int

	defaultDeviceId    string
	defaultMeasurement string
	defaultDataType    string
	timestampPrecision string
}

func init() {
	err := service.RegisterBatchOutput(
		"iotdb",
		outputConfigSpec(),
		constructOutput)
	if err != nil {
		panic(err)
	}
}

func outputConfigSpec() *service.ConfigSpec {
	return service.NewConfigSpec().
		Summary("Inserts items into specified IoTDB timeseries.").
		Description(service.OutputPerformanceDocs(true, true)).
		Categories("Services").
		Fields(connectFields()...).
		Fields(recordFields()...).
		Fields(
			service.NewOutputMaxInFlightField(),
		)
	//supported metadata for single record
	// - device_id
	// - measurement
	// - data_type
	// - timestamp
	// - value
	//supported metadata for multiple records
	// - measurement_array
	// - data_type_array
	// - value_array
}

func constructOutput(conf *service.ParsedConfig, mgr *service.Resources) (out service.BatchOutput, batchPol service.BatchPolicy, mif int, err error) {
	if mif, err = conf.FieldMaxInFlight(); err != nil {
		return
	}
	out, err = newOutputWriter(conf, mgr)
	return
}

func newOutputWriter(conf *service.ParsedConfig, mgr *service.Resources) (*iotdbWriter, error) {
	// var count int
	// var err error
	// if count, err = conf.FieldInt(); err != nil {
	// 	return nil, err
	// }
	var (
		host, port, username, password                      string
		nodeUrls                                            []string
		connTimeoutMs, connRetryMax, maxPoolSize            int
		deviceId, measurement, dataType, timestampPrecision string
		err                                                 error
	)
	if urls, err := conf.FieldStringList(fieldNodeUrls); err != nil {
		return nil, err
	} else {
		if len(urls) == 0 {
			return nil, fmt.Errorf("no urls provided")
		}
		if len(urls) == 1 {
			url := urls[0]
			ind := strings.Index(url, ":")
			if ind > -1 {
				host = url[:ind]
				port = url[ind+1:]
			}
		} else {
			nodeUrls = make([]string, 0, len(urls))
			for _, url := range urls {
				nodeUrls = append(nodeUrls, url)
			}
		}
	}
	if username, err = conf.FieldString(fieldDBUsername); err != nil {
		return nil, err
	}
	if password, err = conf.FieldString(fieldDBPassword); err != nil {
		return nil, err
	}
	if connectTimeout, err := conf.FieldDuration(fieldConnectTimeout); err != nil {
		return nil, err
	} else {
		connTimeoutMs = int(connectTimeout.Milliseconds())
	}
	if connRetryMax, err = conf.FieldInt(fieldConnectRetryMax); err != nil {
		return nil, err
	}
	if maxPoolSize, err = conf.FieldInt(fieldPoolSize); err != nil {
		return nil, err
	}
	if deviceId, err = conf.FieldString(fieldDeviceId); err != nil {
		return nil, err
	}
	if measurement, err = conf.FieldString(fieldMeasurement); err != nil {
		return nil, err
	}
	if dataType, err = conf.FieldString(fieldDataType); err != nil {
		return nil, err
	}
	if timestampPrecision, err = conf.FieldString(fieldTimestampPrecision); err != nil {
		return nil, err
	}

	poolConfig := &client.PoolConfig{
		Host:            host,
		Port:            port,
		NodeUrls:        nodeUrls,
		UserName:        username,
		Password:        password,
		ConnectRetryMax: connRetryMax,
	}

	return &iotdbWriter{
		log:              mgr.Logger(),
		poolConfig:       poolConfig,
		connectTimeoutMs: connTimeoutMs,
		maxPoolSize:      maxPoolSize,

		defaultDeviceId:    deviceId,
		defaultMeasurement: measurement,
		defaultDataType:    dataType,
		timestampPrecision: timestampPrecision,
	}, nil
}

func (writer *iotdbWriter) Connect(ctx context.Context) error {
	pool := client.NewSessionPool(writer.poolConfig, writer.maxPoolSize, writer.connectTimeoutMs, writer.connectTimeoutMs, false)
	// try to get session to verify connection
	session, err := pool.GetSession()
	defer pool.PutBack(session)
	if err != nil {
		writer.log.Errorf("failed to get session: %v", err)
		return err
	}

	writer.sessionPool = &pool
	return nil
}

func (writer *iotdbWriter) WriteBatch(ctx context.Context, batch service.MessageBatch) error {
	var (
		deviceIds    = []string{}
		measurements = [][]string{}
		dataTypes    = [][]client.TSDataType{}
		values       = [][]interface{}{}
		timestamps   = []int64{}
	)
	pool := writer.sessionPool
	if pool == nil {
		writer.log.Error("failed to get session pool")
		return service.ErrNotConnected
	}
	session, err := pool.GetSession()
	defer pool.PutBack(session)
	if err != nil {
		writer.log.Errorf("failed to get session: %v", err)
		return service.ErrNotConnected
	}

	now := time.Now().UnixNano()
	switch writer.timestampPrecision {
	case "us":
		now = now / 1000
	case "ms":
		now = now / 1000000
	}

	err = batch.WalkWithBatchedErrors(func(i int, msg *service.Message) error {
		var deviceId string
		var timestamp int64
		var valuesArray []interface{}
		var measurementArray []string
		var dataTypeArray []client.TSDataType
		deviceIdAny, ok := msg.MetaGetMut(metaDeviceId)
		if ok {
			if deviceId, ok = deviceIdAny.(string); !ok {
				deviceId = writer.defaultDeviceId
			}
		} else {
			deviceId = writer.defaultDeviceId
		}
		timestampAny, ok := msg.MetaGetMut(metaTimestamp)
		if ok {
			if timestamp, ok = timestampAny.(int64); !ok {
				timestamp = now
			}
		} else {
			timestamp = now
		}

		isArray := false
		valuesAny, ok := msg.MetaGetMut(metaValueArray)
		if ok {
			valuesArray, ok = valuesAny.([]any)
			if ok {
				isArray = true
			}
		} else {
			valueAny, ok := msg.MetaGetMut(metaValue)
			if ok {
				valuesArray = []interface{}{valueAny}
			}
		}
		if valuesArray == nil {
			content, err := msg.AsBytes()
			if err != nil {
				return err
			}
			valuesArray = []interface{}{string(content)}
		}

		if isArray {
			measurementsAny, ok := msg.MetaGetMut(metaMeasurementArray)
			useDefault := true
			if ok {
				measurementArray, ok = measurementsAny.([]string)
				if ok {
					useDefault = false
				}
			}
			if useDefault { //expected array but not found, use default measurement
				measurementArray = []string{writer.defaultMeasurement}
			}

			dataTypesAny, ok := msg.MetaGetMut(metaDataTypeArray)
			useDefault = true
			if ok {
				dataTypeArray, ok = dataTypesAny.([]client.TSDataType)
				if ok {
					useDefault = false
				}
			}
			if useDefault { //expected array but not found, use default data type
				dType, err := parseDataType(writer.defaultDataType)
				if err != nil {
					return err
				}
				dataTypeArray = []client.TSDataType{dType}
			}
		} else {
			measurementAny, ok := msg.MetaGetMut(metaMeasurement)
			useDefault := true
			if ok {
				measurement, ok := measurementAny.(string)
				if ok {
					measurementArray = []string{measurement}
					useDefault = false
				}
			}
			if useDefault { //expected meta but not found, use default measurement
				measurementArray = []string{writer.defaultMeasurement}
			}

			dataTypeAny, ok := msg.MetaGetMut(metaDataType)
			useDefault = true
			if ok {
				dataType, ok := dataTypeAny.(client.TSDataType)
				if ok {
					dataTypeArray = []client.TSDataType{dataType}
					useDefault = false
				}
			}

			if useDefault { //expected meta but not found, use default data type
				dType, err := parseDataType(writer.defaultDataType)
				if err != nil {
					return err
				}
				dataTypeArray = []client.TSDataType{dType}
			}
		}

		deviceIds = append(deviceIds, deviceId)
		measurements = append(measurements, measurementArray)
		dataTypes = append(dataTypes, dataTypeArray)
		values = append(values, valuesArray)
		timestamps = append(timestamps, timestamp)

		return nil
	})

	if err != nil {
		writer.log.Errorf("failed to compose records from input messages: %v", err)
		return err
	}

	status, err := session.InsertRecords(deviceIds, measurements, dataTypes, values, timestamps)
	if err != nil {
		writer.log.Errorf("failed to insert records: %v", err)
		return err
	}
	err = client.VerifySuccess(status)
	if err != nil {
		writer.log.Errorf("failed to insert records: %v", err)
		return err
	}

	return nil
}

func (writer *iotdbWriter) Close(context.Context) error {
	pool := writer.sessionPool
	if pool != nil {
		pool.Close()
	}
	return nil
}

func connectFields() []*service.ConfigField {
	return []*service.ConfigField{
		service.NewStringListField(fieldNodeUrls).
			Description("A list of URLs to connect to. If an item of the list contains commas it will be expanded into multiple URLs.").
			Example([]string{"127.0.0.1:6667"}),
		service.NewStringField(fieldDBUsername).
			Description("The username to connect to the IoTDB database.").
			Default(""),
		service.NewStringField(fieldDBPassword).
			Description("The password to connect to the IoTDB database.").
			Default("").
			Secret(),
		service.NewDurationField(fieldConnectTimeout).
			Description("The maximum amount of time to wait in order to establish an IoTDB connection.").
			Default("10s").
			Examples("1s", "500ms"),
		service.NewIntField(fieldConnectRetryMax).
			Description("The maximum number of retries to establish an IoTDB connection.").
			Default(-1),
		service.NewIntField(fieldPoolSize).
			Description("The maximum size of the session pool.").
			Default(-1),
	}
}

func recordFields() []*service.ConfigField {
	return []*service.ConfigField{
		service.NewStringField(fieldDeviceId).
			Description("The default device id."),
		service.NewStringField(fieldMeasurement).
			Description("The default measurement."),
		service.NewStringField(fieldDataType).
			Description("The default data type. Supported types are: BOOLEAN, INT32, INT64, FLOAT, DOUBLE, STRING, TEXT, TIMESTAMP, DATE, BLOB").
			Default("STRING"), //TODO LintRule
		service.NewStringField(fieldTimestampPrecision).
			Description("The precision of the timestamp. Supported precisions are: ms, us, ns").
			Default("ms"), //TODO LintRule
	}
}

func parseDataType(typ string) (client.TSDataType, error) {
	switch typ {
	case "BOOLEAN":
		return client.BOOLEAN, nil
	case "INT32":
		return client.INT32, nil
	case "INT64":
		return client.INT64, nil
	case "FLOAT":
		return client.FLOAT, nil
	case "DOUBLE":
		return client.DOUBLE, nil
	case "STRING":
		return client.STRING, nil
	case "TEXT":
		return client.TEXT, nil
	case "TIMESTAMP":
		return client.TIMESTAMP, nil
	case "DATE":
		return client.DATE, nil
	case "BLOB":
		return client.BLOB, nil
	default:
		return client.UNKNOWN, fmt.Errorf("unsupported data type: %s", typ)
	}
}
