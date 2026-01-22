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

	fieldTable   = "table"
	fieldColumns = "columns"
	// fieldColumnTypes      = "column_types"
	// fieldPartitionColumns = "partition_columns"
	// fieldSortColumns      = "sort_columns"
	fieldColumnName        = "name"
	fieldColumnType        = "type"
	fieldIsPartitionColumn = "is_partition_column"
	fieldIsSortColumn      = "is_sort_column"

	DEFAULT_CONN_TIMEOUT   = 10 * time.Second
	DEFAULT_RETRY_MAX      = 3
	DEFAULT_POOL_SIZE      = 10
	DEFAULT_LB_ENABLED     = false
	DEFAULT_PARTITION_TYPE = "VALUE"
	DEFAULT_ENGINE         = "TSDB"

	DEFAULT_COL_TIMESTAMP  = "timestamp"
	DEFAULT_COL_VALUE      = "value"
	DEFAULT_COL_VALUE_TYPE = "string"
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
}

type tableMeta struct {
	name             string
	columns          []string
	columnTypes      []string
	partitionColumns []string
	sortColumns      []string
}

type dolphindbPreChecker struct {
	log *service.Logger

	poolConfig *api.PoolOption
}

func init() {
	err := service.RegisterBatchOutput(
		"dolphindb",
		outputConfigSpec(),
		constructOutput)
	if err != nil {
		panic(err)
	}
	err = service.RegisterPreChecker(
		"dolphindb",
		"output",
		outputConfigSpec(),
		constructPreChecker)
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
			service.NewIntField("max_in_flight").
				Description("The maximum number of messages to have in flight at a given time. Increase this to improve throughput.").
				Default(64).
				Advanced(),
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

func getConnectFields(conf *service.ParsedConfig, forChecker bool) (*api.PoolOption, error) {
	var (
		address, username, password string
		connectTimeout              time.Duration
		connRetryMax, poolSize      int
		reconnect, lbEnabled        bool
		lbAddresses                 []string
		err                         error
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
	if forChecker {
		connectTimeout = DEFAULT_CONN_TIMEOUT
	} else {
		if connectTimeout, err = conf.FieldDuration(fieldConnectTimeout); err != nil {
			connectTimeout = DEFAULT_CONN_TIMEOUT
		}
	}

	if forChecker {
		connRetryMax = 0
	} else {
		if connRetryMax, err = conf.FieldInt(fieldConnectRetryMax); err != nil {
			connRetryMax = DEFAULT_RETRY_MAX
		}
	}
	if connRetryMax == 0 {
		reconnect = false
	} else {
		reconnect = true
	}

	if forChecker {
		poolSize = 1
	} else {
		if poolSize, err = conf.FieldInt(fieldPoolSize); err != nil {
			poolSize = DEFAULT_POOL_SIZE
		}
	}

	if lbEnabled, err = conf.FieldBool(fieldLoadBalanceEnabled); err != nil {
		lbEnabled = DEFAULT_LB_ENABLED
	}
	if lbAddresses, err = conf.FieldStringList(fieldLoadBalanceAddresses); err != nil {
		lbAddresses = []string{}
	}

	poolConfig := &api.PoolOption{
		Address:              address,
		UserID:               username,
		Password:             password,
		PoolSize:             poolSize,
		Timeout:              connectTimeout,
		Reconnect:            reconnect,
		LoadBalance:          lbEnabled,
		LoadBalanceAddresses: lbAddresses,
	}
	if connRetryMax < 0 {
		poolConfig.TryReconnectNums = nil
	} else {
		poolConfig.TryReconnectNums = &connRetryMax
	}
	return poolConfig, nil
}

func newOutputWriter(conf *service.ParsedConfig, mgr *service.Resources) (*dolphindbWriter, error) {
	var (
		directory, partitionType, partitionScheme, engine string
		columns, columnTypes, partitionCols, sortCols     []string
		table                                             string
	)

	poolConfig, err := getConnectFields(conf, false)
	if err != nil {
		return nil, err
	}

	if directory, err = conf.FieldString(fieldDBDirectory); err != nil {
		return nil, err
	}
	if partitionType, err = conf.FieldString(fieldPartitionType); err != nil {
		partitionType = DEFAULT_PARTITION_TYPE
	}
	if partitionScheme, err = conf.FieldString(fieldPartitionScheme); err != nil {
		now := time.Now()
		layout := "2006.01.02"
		start := now.Format(layout)
		end := now.AddDate(1, 0, -1).Format(layout)
		partitionScheme = fmt.Sprintf("%s..%s", start, end)
	}
	if engine, err = conf.FieldString(fieldEngine); err != nil {
		engine = DEFAULT_ENGINE
	}
	if table, err = conf.FieldString(fieldTable); err != nil {
		return nil, err
	}
	if cols, err := conf.FieldObjectList(fieldColumns); err != nil || len(cols) == 0 {
		columns = []string{DEFAULT_COL_TIMESTAMP, DEFAULT_COL_VALUE}
		columnTypes = []string{DEFAULT_COL_TIMESTAMP, DEFAULT_COL_VALUE_TYPE}
		partitionCols = []string{DEFAULT_COL_TIMESTAMP}
		sortCols = []string{DEFAULT_COL_TIMESTAMP}
	} else {
		for _, col := range cols {
			name, err := col.FieldString(fieldColumnName)
			if err != nil {
				return nil, err
			} else {
				columns = append(columns, name)
			}
			if typ, err := col.FieldString(fieldColumnType); err != nil {
				return nil, err
			} else {
				columnTypes = append(columnTypes, typ)
			}
			if partition, err := col.FieldBool(fieldIsPartitionColumn); err != nil {
				return nil, err
			} else if partition {
				partitionCols = append(partitionCols, name)
			}
			if sort, err := col.FieldBool(fieldIsSortColumn); err != nil {
				return nil, err
			} else if sort {
				sortCols = append(sortCols, name)
			}
		}
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

func constructPreChecker(conf *service.ParsedConfig, mgr *service.Resources) (checker service.PreChecker, err error) {
	poolConfig, err := getConnectFields(conf, true)
	if err != nil {
		return nil, err
	}

	return &dolphindbPreChecker{
		log:        mgr.Logger(),
		poolConfig: poolConfig,
	}, nil
}

func (writer *dolphindbWriter) Connect(ctx context.Context) error {
	pool, err := connect(writer.poolConfig, false)
	if err != nil {
		writer.log.Errorf("failed to create connection pool: %v", err)
		return err
	}
	writer.connectionPool = pool

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

		if !useDefault {
			columnTypesAny, ok := msg.MetaGetMut("column_types")
			if !ok {
				return fmt.Errorf("invalid column_types meta")
			}
			if columnTypes, ok = columnTypesAny.([]string); !ok {
				return fmt.Errorf("invalid column_types meta")
			}

			partitionColsAny, ok := msg.MetaGetMut("partition_columns")
			if !ok {
				return fmt.Errorf("invalid partition_columns meta")
			}
			if partitionCols, ok = partitionColsAny.([]string); !ok {
				return fmt.Errorf("invalid partition_columns meta")
			}

			sortColsAny, ok := msg.MetaGetMut("sort_columns")
			if !ok {
				return fmt.Errorf("invalid sort_columns meta")
			}
			if sortCols, ok = sortColsAny.([]string); !ok {
				return fmt.Errorf("invalid sort_columns meta")
			}
		} else {
			columnTypes = writer.defaultColumnTypes
			partitionCols = writer.defaultPartitionColumns
			sortCols = writer.defaultSortColumns
		}
		if len(columns) != len(columnTypes) {
			return fmt.Errorf("the length of columns and column types do not match")
		}

		if !useDefault {
			valuesAny, ok = msg.MetaGetMut("values")
			if !ok {
				return fmt.Errorf("invalid values meta")
			}
			if values, ok = valuesAny.([]interface{}); !ok {
				return fmt.Errorf("invalid values meta")
			}
		} else {
			if len(columns) == 2 {
				if columns[0] == DEFAULT_COL_TIMESTAMP && columnTypes[1] == DEFAULT_COL_VALUE_TYPE {
					timestamp := time.Now()
					val, err := msg.AsBytes()
					if err != nil {
						return err
					}
					values = []interface{}{timestamp, string(val)}
				}
			}
		}
		if len(columns) != len(values) {
			return fmt.Errorf("the length of columns and values do not match")
		}

		// if useDefault {
		// 	valuesAny, err = msg.AsStructured()
		// 	if err != nil {
		// 		return err
		// 	}
		// 	if values, ok = valuesAny.([]interface{}); !ok {
		// 		return fmt.Errorf("invalid values format")
		// 	}
		// }

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

func (checker *dolphindbPreChecker) Check() error {
	_, err := connect(checker.poolConfig, true)
	if err != nil {
		checker.log.Errorf("failed to create connection pool: %v", err)
		return err
	}
	fmt.Println("!!Connect Successfully")
	return nil
}

func connect(opt *api.PoolOption, closable bool) (*api.DBConnectionPool, error) {
	var ctx context.Context
	var cancel context.CancelFunc

	if opt.Timeout < 0 {
		ctx = context.Background()
		cancel = func() {}
	} else {
		ctx, cancel = context.WithTimeout(context.Background(), opt.Timeout)
	}
	defer cancel()

	result := make(chan struct {
		pool *api.DBConnectionPool
		err  error
	}, 1)
	go func() {
		pool, err := api.NewDBConnectionPool(opt)
		if err == nil {
			// test connection
			err = execTask(pool, `version()`)
		}
		result <- struct {
			pool *api.DBConnectionPool
			err  error
		}{pool, err}
		if closable {
			defer func() {
				if pool != nil {
					pool.Close()
				}
			}()
		}
	}()

	select {
	case res := <-result:
		return res.pool, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("connection timeout after %v", opt.Timeout)
	}
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
			Default(DEFAULT_CONN_TIMEOUT.String()).
			Optional().
			Examples("1s", "500ms"),
		service.NewIntField(fieldConnectRetryMax).
			Description("The maximum number of retries to establish a DolphinDB connection.").
			Default(DEFAULT_RETRY_MAX).
			Optional(),
		service.NewIntField(fieldPoolSize).
			Description("The size of the connection pool.").
			Default(DEFAULT_POOL_SIZE).
			Optional(),
		service.NewBoolField(fieldLoadBalanceEnabled).
			Description("Whether to enable load balancing.").
			Default(DEFAULT_LB_ENABLED).
			Advanced(),
		service.NewStringListField(fieldLoadBalanceAddresses).
			Description("A list of load balance addresses.").
			Advanced().
			Example([]string{"127.0.0.1:8849"}),
	}
}

func recordFields() []*service.ConfigField {
	// columnField := service.NewObjectField("column", service.NewStringField("name"), service.NewStringField("type"))

	return []*service.ConfigField{
		service.NewStringField(fieldDBDirectory).
			Description("The directory to store the database."),
		service.NewStringEnumField(fieldPartitionType, "SEQ", "RANGE", "HASH", "VALUE", "LIST", "COMPO").
			Description("The partition type. Supported types are: SEQ, RANGE, HASH, VALUE, LIST, COMPO").
			Default(DEFAULT_PARTITION_TYPE).
			Advanced(),
		service.NewStringField(fieldPartitionScheme).
			Optional().
			Advanced().
			Description("The partition scheme."),
		service.NewStringEnumField(fieldEngine, "OLAP", "TSDB", "IMOLTP", "IOTDB", "'PKEY'").
			Description("The storage engine. Supported engines are: OLAP, TSDB, IMOLTP, IOTDB, 'PKEY'.").
			Default(DEFAULT_ENGINE).
			Advanced(),
		service.NewStringField(fieldTable).
			Description("The default table name."),
		service.NewObjectListField(fieldColumns,
			service.NewStringField(fieldColumnName),
			service.NewStringField(fieldColumnType),
			service.NewBoolField(fieldIsPartitionColumn),
			service.NewBoolField(fieldIsSortColumn),
		).
			Optional().
			Advanced().
			Description("The default columns of the table"),
		// service.NewStringListField(fieldColumns).
		// 	Advanced().
		// 	Description("The default columns of the table."),
		// service.NewStringListField(fieldColumnTypes).
		// 	Advanced().
		// 	Description("The default type for each column."),
		// service.NewStringListField(fieldPartitionColumns).
		// 	Advanced().
		// 	Description("The default partition columns of the table."),
		// service.NewStringListField(fieldSortColumns).
		// 	Advanced().
		// 	Description("The default sort columns of the table."),
	}
}

func buildScript(name, templ string, param map[string]any) (string, error) {
	tmpl := template.Must(template.New(name).Parse(templ))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, param); err != nil {
		return "", err
	}
	fmt.Println(buf.String())
	return buf.String(), nil
}

func execTask(pool *api.DBConnectionPool, script string) error {
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
	return nil
}

func execTaskWithResult(pool *api.DBConnectionPool, script string) error {
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
	return execTask(writer.connectionPool, `version()`)
}

// func doExecTask(pool *api.DBConnectionPool, task *api.Task) error {
// 	err := pool.Execute([]*api.Task{task})
// 	if err != nil {
// 		return err
// 	}
// 	if !task.IsSuccess() {
// 		err := task.GetError()
// 		return err
// 	}
// 	return nil
// }

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
	err = execTaskWithResult(writer.connectionPool, script)
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
	err = execTaskWithResult(writer.connectionPool, script)
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
