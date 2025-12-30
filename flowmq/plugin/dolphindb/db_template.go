package dolphindb

const dbCreateTmpl = `
directory="{{ .Directory }}"
partitionType={{ .PartitionType }}
partitionScheme={{ .PartitionScheme }}
engine="{{ .Engine }}"
//0: successful; 1: error
code = 0
try {
	if (directory == "") {
		throw "directory must be provided"
	}
	database(directory=directory, partitionType=partitionType, partitionScheme=partitionScheme, engine=engine)
	if (!existsDatabase(directory)) {
		throw "failed to create database " + directory
	}
	msg = "successful"
} catch (ex) {
	code = 1
	msg = "error: " + ex
}
dict(['code', 'message'], [code, msg])
`

const tableCreateTmpl = `
directory="{{ .Directory }}"
tableName="{{ .TableName }}"
partitions={{ .Partitions }}
sorts={{ .Sorts }}
//0: successful; 1: error
code = 0
try {
	if (tableName == "") {
		throw "table name must be provided"
	}
	if (!existsTable(directory, tableName)) {
		schema = table({{ range $i, $colType := .ColumnTypes }}{{ if $i }}, {{ end }}{{ $colType }}(1..0) as {{ index $.Columns $i }}{{ end }})
		db = database(directory)
		if (sorts == "") {
			db.createPartitionedTable(table=schema, tableName=tableName, partitionColumns=partitions)
		} else {
			db.createPartitionedTable(table=schema, tableName=tableName, partitionColumns=partitions, sortColumns=sorts)
		}
		if (!existsTable(directory, tableName)) {
			throw "failed to create table " + tableName
		}
	}	
	msg = "successful"
} catch (ex) {
	code = 1
	msg = "" + ex
}
dict(['code', 'message'], [code, msg])
`
