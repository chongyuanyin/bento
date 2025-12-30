package dolphindb

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"text/template"
	"time"

	"github.com/dolphindb/api-go/v3/api"
	"github.com/dolphindb/api-go/v3/model"
	"github.com/warpstreamlabs/bento/public/service"
)

const (
	// Connection Fields
	fieldAddress              = "url"
	fieldDBUsername           = "username"
	fieldDBPassword           = "password"
	fieldConnectTimeout       = "connect_timeout"
	fieldConnectRetryMax      = "connect_retry_max"
	fieldPoolSize             = "pool_size"
	fieldLoadBalanceEnabled   = "lb_enabled"
	fieldLoadBalanceAddresses = "lb_addresses"

	fieldDBDirectory     = "directory"
	fieldPartitionType   = "partition_type"
	fieldPartitionScheme = "partition_scheme"
	fieldEngine          = "engine"

	fieldTable            = "table"
	fieldColumns          = "columns"
	fieldColumnTypes      = "column_types"
	fieldPartitionColumns = "partition_columns"
	fieldSortColumns      = "sort_columns"

	// fieldDeviceId           = "device_id"
	// fieldMeasurement        = "measurement"
	// fieldDataType           = "data_type"
	// fieldTimestampPrecision = "timestamp_precision"

	// metaDeviceId         = "device_id"
	// metaMeasurement      = "measurement"
	// metaDataType         = "data_type"
	// metaTimestamp        = "timestamp"
	// metaValue            = "value"
	// metaMeasurementArray = "measurement_array"
	// metaDataTypeArray    = "data_type_array"
	// metaValueArray       = "value_array"
)

type dolphindbWriter struct {
	log *service.Logger

	poolConfig     *api.PoolOption
	connectionPool *api.DBConnectionPool

	directory       string
	partitionType   string
	partitionScheme string
	engine          string

	defaultTable            string
	defaultColumns          []string
	defaultColumnTypes      []string
	defaultPartitionColumns []string
	defaultSortColumns      []string

	// connectTimeoutMs int
	// maxPoolSize      int

	// defaultDeviceId    string
	// defaultMeasurement string
	// defaultDataType    string
	// timestampPrecision string
}

// type dbTaskParam struct {
// 	Directory       string
// 	PartitionType   string
// 	PartitionScheme string
// 	Engine          string
// }

// type tableTaskParam struct {
// 	Directory   string
// 	TableName   string
// 	Columns     []string
// 	ColumnTypes []string
// 	Partitions  string
// }

type tableMeta struct {
	name             string
	columns          []string
	columnTypes      []string
	partitionColumns []string
	sortColumns      []string
}

func init() {
	fmt.Println("!!init")
	err := service.RegisterBatchOutput(
		"dolphindb",
		outputConfigSpec(),
		constructOutput)
	if err != nil {
		panic(err)
	}
}

func outputConfigSpec() *service.ConfigSpec {
	return service.NewConfigSpec().
		Summary("Inserts items into DolphinDB.").
		Description(service.OutputPerformanceDocs(true, true)).
		Categories("Services").
		Fields(connectFields()...).
		Fields(recordFields()...).
		Fields(
			service.NewOutputMaxInFlightField(),
		)
	//supported metadata
	// - table
	// - columns
	// - partition_columns
	// - values
}

func constructOutput(conf *service.ParsedConfig, mgr *service.Resources) (out service.BatchOutput, batchPol service.BatchPolicy, mif int, err error) {
	if mif, err = conf.FieldMaxInFlight(); err != nil {
		return
	}
	out, err = newOutputWriter(conf, mgr)
	return
}

func newOutputWriter(conf *service.ParsedConfig, mgr *service.Resources) (*dolphindbWriter, error) {
	var (
		address, username, password, directory, partitionType, partitionScheme, engine string
		connectTimeout                                                                 time.Duration
		connRetryMax, poolSize                                                         int
		lbEnabled                                                                      bool
		lbAddresses, columns, columnTypes, partitionCols, sortCols                     []string
		table                                                                          string
		err                                                                            error
	)

	if address, err = conf.FieldString(fieldAddress); err != nil {
		return nil, err
	}
	if username, err = conf.FieldString(fieldDBUsername); err != nil {
		return nil, err
	}
	if password, err = conf.FieldString(fieldDBPassword); err != nil {
		return nil, err
	}
	if connectTimeout, err = conf.FieldDuration(fieldConnectTimeout); err != nil {
		return nil, err
	}
	if connRetryMax, err = conf.FieldInt(fieldConnectRetryMax); err != nil {
		return nil, err
	}
	if poolSize, err = conf.FieldInt(fieldPoolSize); err != nil {
		return nil, err
	}
	if lbEnabled, err = conf.FieldBool(fieldLoadBalanceEnabled); err != nil {
		return nil, err
	}
	if lbAddresses, err = conf.FieldStringList(fieldLoadBalanceAddresses); err != nil {
		return nil, err
	}
	if directory, err = conf.FieldString(fieldDBDirectory); err != nil {
		return nil, err
	}
	if partitionType, err = conf.FieldString(fieldPartitionType); err != nil {
		return nil, err
	}
	if partitionScheme, err = conf.FieldString(fieldPartitionScheme); err != nil {
		return nil, err
	}
	if engine, err = conf.FieldString(fieldEngine); err != nil {
		return nil, err
	}
	if table, err = conf.FieldString(fieldTable); err != nil {
		return nil, err
	}
	if columns, err = conf.FieldStringList(fieldColumns); err != nil {
		return nil, err
	}
	if columnTypes, err = conf.FieldStringList(fieldColumnTypes); err != nil {
		return nil, err
	}
	if partitionCols, err = conf.FieldStringList(fieldPartitionColumns); err != nil {
		return nil, err
	}
	if sortCols, err = conf.FieldStringList(fieldSortColumns); err != nil {
		return nil, err
	}

	poolConfig := &api.PoolOption{
		Address:              address,
		UserID:               username,
		Password:             password,
		PoolSize:             poolSize,
		Timeout:              connectTimeout,
		Reconnect:            true,
		TryReconnectNums:     &connRetryMax,
		LoadBalance:          lbEnabled,
		LoadBalanceAddresses: lbAddresses,
	}

	return &dolphindbWriter{
		log:                     mgr.Logger(),
		poolConfig:              poolConfig,
		directory:               directory,
		partitionType:           partitionType,
		partitionScheme:         partitionScheme,
		engine:                  engine,
		defaultTable:            table,
		defaultColumns:          columns,
		defaultColumnTypes:      columnTypes,
		defaultPartitionColumns: partitionCols,
		defaultSortColumns:      sortCols,
	}, nil
}

func (writer *dolphindbWriter) Connect(ctx context.Context) error {
	fmt.Println("!!Connect")
	pool, err := api.NewDBConnectionPool(writer.poolConfig)
	if err != nil {
		writer.log.Errorf("failed to create connection pool: %v", err)
		return err
	}
	writer.connectionPool = pool

	// test connection
	err = writer.testDbAvailable()
	if err != nil {
		return err
	}

	// create database if needed
	err = writer.createDatabase()
	if err != nil {
		return err
	}

	return nil
}

func (writer *dolphindbWriter) WriteBatch(ctx context.Context, batch service.MessageBatch) error {
	fmt.Println("!!WriteBatch")
	var (
		table                                         string
		columns, columnTypes, partitionCols, sortCols []string
		valuesAny                                     any
		values                                        []interface{}
		err                                           error
		tableMetas                                    map[string]tableMeta          = make(map[string]tableMeta)
		tableRecords                                  map[string][][]model.DataType = make(map[string][][]model.DataType)
	)

	pool := writer.connectionPool
	if pool == nil {
		writer.log.Error("failed to get connection pool")
		return service.ErrNotConnected
	}

	err = batch.WalkWithBatchedErrors(func(i int, msg *service.Message) error {
		tableAny, ok := msg.MetaGetMut("table")
		if ok {
			if table, ok = tableAny.(string); !ok {
				table = writer.defaultTable
			}
		} else {
			table = writer.defaultTable
		}

		useDefault := true
		columnsAny, ok := msg.MetaGetMut("columns")
		if ok {
			if columns, ok = columnsAny.([]string); ok {
				useDefault = false
			}
		}
		if useDefault {
			columns = writer.defaultColumns
		}

		useDefault = true
		columnTypesAny, ok := msg.MetaGetMut("column_types")
		if ok {
			if columnTypes, ok = columnTypesAny.([]string); ok {
				useDefault = false
			}
		}
		if useDefault {
			columnTypes = writer.defaultColumnTypes
		}

		if len(columns) != len(columnTypes) {
			return fmt.Errorf("the length of columns and column types do not match")
		}

		useDefault = true
		partitionColsAny, ok := msg.MetaGetMut("partition_columns")
		if ok {
			if partitionCols, ok = partitionColsAny.([]string); ok {
				useDefault = false
			}
		}
		if useDefault {
			partitionCols = writer.defaultPartitionColumns
		}

		useDefault = true
		sortColsAny, ok := msg.MetaGetMut("sort_columns")
		if ok {
			if sortCols, ok = sortColsAny.([]string); ok {
				useDefault = false
			}
		}
		if useDefault {
			sortCols = writer.defaultSortColumns
		}

		useDefault = true
		valuesAny, ok = msg.MetaGetMut("values")
		if ok {
			if values, ok = valuesAny.([]interface{}); ok {
				useDefault = false
			}
		}
		if useDefault {
			valuesAny, err = msg.AsStructured()
			if err != nil {
				return err
			}
			if values, ok = valuesAny.([]interface{}); !ok {
				return fmt.Errorf("invalid values format")
			}
		}
		if len(columns) != len(values) {
			return fmt.Errorf("the length of columns and values do not match")
		}

		meta := tableMeta{
			name:             table,
			columns:          columns,
			columnTypes:      columnTypes,
			partitionColumns: partitionCols,
			sortColumns:      sortCols,
		}
		existMeta, ok := tableMetas[table]
		if !ok {
			tableMetas[table] = meta
			// create table if not exist
			err = writer.createTable(meta)
			if err != nil {
				return err
			}
		} else {
			// validate columns and column types
			if !compareTableMeta(existMeta, meta) {
				return fmt.Errorf("table %s should always have same columns and column types", table)
			}
		}

		dataTypes := tableRecords[table]
		dataTypes, err = buildRecords(values, columnTypes, dataTypes)
		if err != nil {
			return err
		}
		tableRecords[table] = dataTypes

		return nil
	})

	if err != nil {
		writer.log.Errorf("failed to compose records from input messages: %v", err)
		return err
	}

	for table, dataTypes := range tableRecords {
		meta, ok := tableMetas[table]
		if !ok {
			err = fmt.Errorf("failed to get column info for table %s", table)
			writer.log.Error(err.Error())
			return err
		}

		appenderOpt := &api.PartitionedTableAppenderOption{
			Pool:      pool,
			DBPath:    writer.directory,
			TableName: table,
		}
		if len(meta.partitionColumns) > 0 {
			appenderOpt.PartitionCol = meta.partitionColumns[0]
		}

		appender, err := api.NewPartitionedTableAppender(appenderOpt)
		if err != nil {
			writer.log.Errorf("failed to create appender: %v", err)
			return err
		}

		colNames := meta.columns
		colVals := make([]*model.Vector, len(colNames))
		for i, dt := range dataTypes {
			if len(dt) > 0 {
				typ := dt[0].DataType()
				dtList := model.NewDataTypeList(typ, dt)
				colVals[i] = model.NewVector(dtList)
			}
		}
		_, err = appender.Append(model.NewTable(colNames, colVals))
		if err != nil {
			writer.log.Errorf("failed to append records to table %s: %v", table, err)
			return err
		}
		fmt.Printf("!!!inserted: %v\n", colVals)

		// err = appender.Close()
		// if err != nil {
		// 	writer.log.Warn("failed to close appender")
		// }
	}

	return nil
}

func (writer *dolphindbWriter) Close(context.Context) error {
	fmt.Println("!!Close")
	pool := writer.connectionPool
	if pool != nil {
		pool.Close()
	}
	return nil
}

func connectFields() []*service.ConfigField {
	return []*service.ConfigField{
		service.NewStringField(fieldAddress).
			Description("The DolphinDB server address.").
			Example("127.0.0.1:8848"),
		service.NewStringField(fieldDBUsername).
			Description("The username to connect to the DolphinDB database.").
			Default(""),
		service.NewStringField(fieldDBPassword).
			Description("The password to connect to the DolphinDB database.").
			Default("").
			Secret(),
		service.NewDurationField(fieldConnectTimeout).
			Description("The maximum amount of time to wait in order to establish a DolphinDB connection.").
			Default("10s").
			Examples("1s", "500ms"),
		service.NewIntField(fieldConnectRetryMax).
			Description("The maximum number of retries to establish a DolphinDB connection.").
			Default(0),
		service.NewIntField(fieldPoolSize).
			Description("The size of the connection pool.").
			Default(1),
		service.NewBoolField(fieldLoadBalanceEnabled).
			Description("Whether to enable load balancing.").
			Default(false),
		service.NewStringListField(fieldLoadBalanceAddresses).
			Description("A list of load balance addresses.").
			Example([]string{"127.0.0.1:8849"}),
	}
}

func recordFields() []*service.ConfigField {
	// columnField := service.NewObjectField("column", service.NewStringField("name"), service.NewStringField("type"))

	return []*service.ConfigField{
		service.NewStringField(fieldDBDirectory).
			Description("The directory to store the database."),
		service.NewStringField(fieldPartitionType).
			Description("The partition type. Supported types are: SEQ, RANGE, HASH, VALUE, LIST, COMPO").
			Default("VALUE"), //TODO LintRule
		service.NewStringField(fieldPartitionScheme).
			Description("The partition scheme."),
		service.NewStringField(fieldEngine).
			Description("The storage engine. Supported engines are: OLAP, TSDB, IMOLTP, IOTDB, 'PKEY'.").
			Default("TSDB"), //TODO LintRule
		service.NewStringField(fieldTable).
			Description("The default table name."),
		service.NewStringListField(fieldColumns).
			Description("The default columns of the table."),
		service.NewStringListField(fieldColumnTypes).
			Description("The default type for each column."),
		service.NewStringListField(fieldPartitionColumns).
			Description("The default partition columns of the table."),
		service.NewStringListField(fieldSortColumns).
			Description("The default sort columns of the table."),
	}
}

func buildScript(name, templ string, param map[string]any) (string, error) {
	tmpl := template.Must(template.New(name).Parse(templ))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, param); err != nil {
		return "", err
	}
	// fmt.Println(buf.String())
	return buf.String(), nil
}

func execTask(pool *api.DBConnectionPool, script string) error {
	// getnametask := api.Task{Script: "111"}
	// tasks := []*api.Task{&getnametask}
	// err := pool.Execute(tasks)
	// origin_node := tasks[0].GetResult()
	// fmt.Println(origin_node)

	task := &api.Task{
		Script: script,
	}
	err := pool.Execute([]*api.Task{task})
	if err != nil {
		return err
	}
	if !task.IsSuccess() {
		err := task.GetError()
		return err
	}
	rawResult := task.GetResult()
	fmt.Println(rawResult)
	if rawResult == nil {
		return errors.New("task execution result is empty")
	}
	result, ok := rawResult.(*model.Dictionary)
	if !ok {
		return errors.New("task execution result is not a dictionary")
	}
	codeVal, err := result.Get("code")
	if err != nil {
		return err
	}

	code, err := getIntFromDataType(codeVal)
	if err != nil {
		return err
	}

	if code != 0 {
		msgVal, err := result.Get("message")
		if err != nil {
			return err
		}
		msg, err := getStringFromDataType(msgVal)
		if err != nil {
			return err
		}
		return errors.New(msg)
	}

	return nil
}

func (writer *dolphindbWriter) testDbAvailable() error {
	versionTask := &api.Task{
		Script: `version()`,
	}
	err := writer.connectionPool.Execute([]*api.Task{versionTask})
	if err != nil {
		writer.log.Errorf("failed to connect to db: %v", err)
		return err
	}
	if !versionTask.IsSuccess() {
		err := versionTask.GetError()
		writer.log.Errorf("failed to connect to db: %v", err)
		return err
	}
	return nil
}

func (writer *dolphindbWriter) createDatabase() error {
	// create database if needed
	params := map[string]any{
		"Directory":       writer.directory,
		"PartitionType":   writer.partitionType,
		"PartitionScheme": writer.partitionScheme,
		"Engine":          writer.engine,
	}
	script, err := buildScript("createDB", dbCreateTmpl, params)
	if err != nil {
		writer.log.Errorf("failed to generate script for database creation: %v", err)
		return err
	}
	err = execTask(writer.connectionPool, script)
	if err != nil {
		writer.log.Errorf("failed to create database: %v", err)
		return err
	}
	return nil
}

func (writer *dolphindbWriter) createTable(meta tableMeta) error {
	// create partitioned table if needed
	params := map[string]any{
		"Directory":   writer.directory,
		"TableName":   meta.name,
		"Columns":     meta.columns,
		"ColumnTypes": meta.columnTypes,
		"Partitions":  buildVectors(meta.partitionColumns),
		"Sorts":       buildVectors(meta.sortColumns),
	}
	script, err := buildScript("createTable", tableCreateTmpl, params)
	if err != nil {
		writer.log.Errorf("failed to generate script for table creation: %v", err)
		return err
	}
	err = execTask(writer.connectionPool, script)
	if err != nil {
		writer.log.Errorf("failed to create table: %v", err)
		return err
	}
	return nil
}

func compareTableMeta(expect tableMeta, actual tableMeta) bool {
	if len(expect.columns) != len(actual.columns) ||
		len(expect.columnTypes) != len(actual.columnTypes) ||
		len(expect.partitionColumns) != len(actual.partitionColumns) {
		return false
	}

	expectColMap := make(map[string]string)
	actualColMap := make(map[string]string)
	for i, col := range expect.columns {
		expectColMap[col] = expect.columnTypes[i]
	}
	for i, col := range actual.columns {
		actualColMap[col] = actual.columnTypes[i]
	}

	for col, typ := range expectColMap {
		actualTyp, ok := actualColMap[col]
		if !ok {
			return false
		}
		if actualTyp != typ {
			return false
		}
	}

	expectPartition := make(map[string]interface{})
	actualPartition := make(map[string]interface{})
	for _, col := range expect.partitionColumns {
		expectPartition[col] = struct{}{}
	}
	for _, col := range actual.partitionColumns {
		actualPartition[col] = struct{}{}
	}

	for col := range expectPartition {
		if _, ok := actualPartition[col]; !ok {
			return false
		}
	}

	return true
}

func buildRecords(values []interface{}, columnTypes []string, dataTypes [][]model.DataType) ([][]model.DataType, error) {
	if dataTypes == nil {
		dataTypes = make([][]model.DataType, len(values))
	}

	for i, rawVal := range values {
		list := dataTypes[i]
		if list == nil {
			list = []model.DataType{}
		}
		typ, err := getDataTypeFromString(columnTypes[i])
		if err != nil {
			return nil, err
		}
		val, err := renderData(typ, rawVal)
		if err != nil {
			return nil, fmt.Errorf("column data (index %d) does not match type %s: %v", i, columnTypes[i], rawVal)
		}
		dt, err := model.NewDataType(typ, val)
		if err != nil {
			return nil, err
		}
		list = append(list, dt)
		dataTypes[i] = list
	}
	return dataTypes, nil
}

func buildVectors(values []string) string {
	result := ""
	for _, val := range values {
		result += "`" + val
	}
	return result
}
