package swarmdb

import (
	"bytes"
	//"encoding/binary"
	"encoding/json"
	//"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/swarmdb/log"
	"path/filepath"
	"reflect"
	"strconv"
)

func NewSwarmDB(ensPath string, chunkDBPath string) *SwarmDB {
	sd := new(SwarmDB)

	// ownerID, tableName => *Table
	sd.tables = make(map[string]*Table)
	chunkdbFileName := "chunk.db"
	dbChunkStoreFullPath := filepath.Join(chunkDBPath, chunkdbFileName)
	dbchunkstore, err := NewDBChunkStore(dbChunkStoreFullPath)
	if err != nil {
		// TODO: PANIC
		fmt.Printf("NO CHUNK STORE!\n")
	} else {
		sd.dbchunkstore = dbchunkstore
	}

	//default /tmp/ens.db
	ensdbFileName := "ens.db"
	ensdbFullPath := filepath.Join(ensPath, ensdbFileName)
	ens, err := NewENSSimulation(ensdbFullPath)
	if err != nil {
		// TODO: PANIC
		fmt.Printf("NO ENS!\n")
	} else {
		sd.ens = ens
	}

	kaddb, err := NewKademliaDB(dbchunkstore)
	if err != nil {
	} else {
		sd.kaddb = kaddb
	}

	sd.Logger = swarmdblog.NewLogger()

	return sd
}

// DBChunkStore  API
/*
func (self *SwarmDB) RetrieveKDBChunk(u *SWARMDBUser, key []byte) (val []byte, err error) {
	return self.dbchunkstore.RetrieveKChunk(u, key)
}

func (self *SwarmDB) StoreKDBChunk(key []byte, val []byte) (err error) {
	return self.dbchunkstore.StoreKChunk(key, val)
}
*/

func (self *SwarmDB) PrintDBChunk(columnType ColumnType, hashid []byte, c []byte) {
	self.dbchunkstore.PrintDBChunk(columnType, hashid, c)
}

func (self *SwarmDB) RetrieveDBChunk(u *SWARMDBUser, key []byte) (val []byte, err error) {
	val, err = self.dbchunkstore.RetrieveChunk(u, key)
	return val, err
}

func (self *SwarmDB) StoreDBChunk(u *SWARMDBUser, val []byte, encrypted int) (key []byte, err error) {
	key, err = self.dbchunkstore.StoreChunk(u, val, encrypted)
	return key, err
}

// ENSSimulation  API
func (self *SwarmDB) GetRootHash(columnName []byte) (roothash []byte, err error) {
	return self.ens.GetRootHash(columnName)
}

func (self *SwarmDB) StoreRootHash(columnName []byte, roothash []byte) (err error) {
	return self.ens.StoreRootHash(columnName, roothash)
}

// parse sql and return rows in bulk (order by, group by, etc.)
func (self *SwarmDB) QuerySelect(u *SWARMDBUser, query *QueryOption) (rows []Row, err error) {
	table, err := self.GetTable(u, query.TableOwner, query.Table)
	if err != nil {
		return rows, err
	}

	//var rawRows []Row
	colRows, err := self.Scan(u, query.TableOwner, query.Table, table.primaryColumnName, query.Ascending)
	if err != nil {
		return rows, err
	}
	fmt.Printf("\nColRows = [%+v]", colRows)

	//TODO: 2ndary column scans
	/*
		for _, column := range query.RequestColumns {
			if err != nil {
				fmt.Printf("\nError Scanning table [%s] : [%s]", query.Table, err)
				return rows, err
			}
			fmt.Printf("\nQuerySelect scanned rows: %+v\n", colRows)
			fmt.Printf("\nNumber of rows scanned: %d for column [%s]", len(colRows), column.ColumnName)
			for _, colRow := range colRows {
				for _, row := range rawRows {
					fmt.Printf("\nComparing ROW [%v] vs ColRow [%v]", row, colRow)
					if isDuplicateRow(row, colRow) {
						fmt.Printf("QS: found duped row! %+v\n", row)
					} else {
						rawRows = append(rawRows, colRow)
					}
				}
			}
		}
		fmt.Printf("\nNumber of RAW rows returned : %d", len(rawRows))
	*/
	//apply WHERE
	whereRows, err := table.applyWhere(colRows, query.Where)
	if err != nil {
		return rows, err
	}
	fmt.Printf("\nQuerySelect applied where rows: %+v\n", whereRows)

	fmt.Printf("\nNumber of WHERE rows returned : %d", len(whereRows))
	//filter for requested columns
	for _, row := range whereRows {
		fmt.Printf("QS b4 filterRowByColumns row: %+v\n", row)
		fRow := filterRowByColumns(&row, query.RequestColumns)
		fmt.Printf("QS after filterRowByColumns row: %+v\n", fRow)
		if len(fRow.Cells) > 0 {
			rows = append(rows, fRow)
		}
	}
	fmt.Printf("\nNumber of FINAL rows returned : %d", len(rows))

	//TODO: Put it in order for Ascending/GroupBy
	fmt.Printf("\nQS returning: %+v\n", rows)
	return rows, nil
}

//Insert is for adding new data to the table
//example: 'INSERT INTO tablename (col1, col2) VALUES (val1, val2)
func (self *SwarmDB) QueryInsert(u *SWARMDBUser, query *QueryOption) (err error) {

	table, err := self.GetTable(u, query.TableOwner, query.Table)
	if err != nil {
		return err
	}
	for _, row := range query.Inserts {
		//check if primary column exists in Row
		if _, ok := row.Cells[table.primaryColumnName]; !ok {
			return fmt.Errorf("Insert row %+v needs primary column '%s' value", row, table.primaryColumnName)
		}
		//check if Row already exists
		//TODO: convertJSONValueToKey ERROR
		convertedKey, err := convertJSONValueToKey(table.columns[table.primaryColumnName].columnType, row.Cells[table.primaryColumnName])
		if err != nil {
			return err
		}
		existingByteRow, err := table.Get(u, convertedKey)
		if err != nil {
			existingRow, _ := table.byteArrayToRow(existingByteRow)
			//TODO: Trying to insert duplicate row error
			return fmt.Errorf("Insert row key %s already exists: %+v", row.Cells[table.primaryColumnName], existingRow)
		}
		//put the new Row in
		err = table.Put(u, row.Cells)
		if err != nil {
			return err
		}
	}

	return nil
}

//Update is for modifying existing data in the table (can use a Where clause)
//example: 'UPDATE tablename SET col1=value1, col2=value2 WHERE col3 > 0'
func (self *SwarmDB) QueryUpdate(u *SWARMDBUser, query *QueryOption) (err error) {

	table, err := self.GetTable(u, query.TableOwner, query.Table)
	if err != nil {
		return err
	}

	//get all rows with Scan, using primary key column
	rawRows, err := self.Scan(u, query.TableOwner, query.Table, table.primaryColumnName, query.Ascending)
	if err != nil {
		return err
	}

	//check to see if Update cols are in pulled set
	for colname, _ := range query.Update {
		if _, ok := table.columns[colname]; !ok {
			//TODO:
			return fmt.Errorf("Update SET column name %s is not in table", colname)
		}
	}

	//apply WHERE clause
	filteredRows, err := table.applyWhere(rawRows, query.Where)
	if err != nil {
		return err
	}

	//set the appropriate columns in filtered set
	for i, row := range filteredRows {
		for colname, value := range query.Update {
			if _, ok := row.Cells[colname]; !ok {
				return fmt.Errorf("Update SET column name %s is not in filtered rows", colname)
			}
			filteredRows[i].Cells[colname] = value
		}
	}

	//put the changed rows back into the table
	for _, row := range filteredRows {
		err := table.Put(u, row.Cells)
		if err != nil {
			return err
		}
	}

	return nil
}

//Delete is for deleting data rows (can use a Where clause, not just a key)
//example: 'DELETE FROM tablename WHERE col1 = value1'
func (self *SwarmDB) QueryDelete(u *SWARMDBUser, query *QueryOption) (err error) {

	table, err := self.GetTable(u, query.TableOwner, query.Table)
	if err != nil {
		return err
	}

	//get all rows with Scan, using Where's specified col
	rawRows, err := self.Scan(u, query.TableOwner, query.Table, query.Where.Left, query.Ascending)
	if err != nil {
		return err
	}

	//apply WHERE clause
	filteredRows, err := table.applyWhere(rawRows, query.Where)
	if err != nil {
		return err
	}

	//delete the selected rows
	for _, row := range filteredRows {
		_, err := table.Delete(u, row.Cells[table.primaryColumnName].(string))
		if err != nil {
			//TODO: expecting a certain error to bubble up notifying of a problem Deleting
			return err
		}
		//if !ok, what should happen?
		//TODO: return appropriate response -- number of records affected
	}
	return nil
}

func (t *Table) assignRowColumnTypes(rows []Row) ([]Row, error) {
	for _, row := range rows {
		for name, value := range row.Cells {
			switch t.columns[name].columnType {
			case CT_INTEGER:
				switch value.(type) {
				case int:
					row.Cells[name] = value.(int)
				case float64:
					row.Cells[name] = int(value.(float64))
				default:
					return rows, fmt.Errorf("TypeConversion Error: value [%v] does not match column type [%v]", value, t.columns[name].columnType)
				}
			case CT_STRING:
				switch value.(type) {
				case string:
					row.Cells[name] = value.(string)
				case int:
					row.Cells[name] = strconv.Itoa(value.(int))
				case float64:
					row.Cells[name] = strconv.FormatFloat(value.(float64), 'E', -1, 64)
				default:
					return rows, fmt.Errorf("TypeConversion Error: value [%v] does not match column type [%v]", value, t.columns[name].columnType)
				}
			case CT_FLOAT:
				switch value.(type) {
				case float64:
					row.Cells[name] = value.(float64)
				case int:
					row.Cells[name] = float64(value.(int))
				default:
					return rows, fmt.Errorf("TypeConversion Error: value [%v] does not match column type [%v]", value, t.columns[name].columnType)
				}
			case CT_BLOB:
				//TODO?
			default:
				return rows, fmt.Errorf("Coltype not found", t.columns[name].columnType)
			}
		}
	}
	return rows, nil
}

//TODO: could overload the operators so this isn't so clunky
//TODO: Blob types
func (t *Table) applyWhere(rawRows []Row, where Where) (outRows []Row, err error) {
	for _, row := range rawRows {
		if _, ok := row.Cells[where.Left]; !ok {
			return outRows, fmt.Errorf("Where clause col %s doesn't exist in table")
		}
		colType := t.columns[where.Left].columnType
		right, err := stringToColumnType(where.Right, colType)
		if err != nil {
			return outRows, err
		}
		fRow := NewRow()
		switch where.Operator {
		case "=":
			switch colType {
			case CT_INTEGER:
				if row.Cells[where.Left].(int) == right.(int) {
					fRow.Cells = row.Cells
				}
			case CT_FLOAT:
				if row.Cells[where.Left].(float64) == right.(float64) {
					fRow.Cells = row.Cells
				}
			case CT_STRING:
				if row.Cells[where.Left].(string) == right.(string) {
					fRow.Cells = row.Cells
				}
			}
		case "<":
			switch colType {
			case CT_INTEGER:
				if row.Cells[where.Left].(int) < right.(int) {
					fRow.Cells = row.Cells
				}
			case CT_FLOAT:
				if row.Cells[where.Left].(float64) < right.(float64) {
					fRow.Cells = row.Cells
				}
			case CT_STRING:
				if row.Cells[where.Left].(string) < right.(string) {
					fRow.Cells = row.Cells
				}
			}
		case "<=":
			switch colType {
			case CT_INTEGER:
				if row.Cells[where.Left].(int) <= right.(int) {
					fRow.Cells = row.Cells
				}
			case CT_FLOAT:
				if row.Cells[where.Left].(float64) <= right.(float64) {
					fRow.Cells = row.Cells
				}
			case CT_STRING:
				if row.Cells[where.Left].(string) <= right.(string) {
					fRow.Cells = row.Cells
				}
			}
		case ">":
			switch colType {
			case CT_INTEGER:
				if row.Cells[where.Left].(int) > right.(int) {
					fRow.Cells = row.Cells
				}
			case CT_FLOAT:
				if row.Cells[where.Left].(float64) > right.(float64) {
					fRow.Cells = row.Cells
				}
			case CT_STRING:
				if row.Cells[where.Left].(string) > right.(string) {
					fRow.Cells = row.Cells
				}
			}
		case ">=":
			switch colType {
			case CT_INTEGER:
				if row.Cells[where.Left].(int) >= right.(int) {
					fRow.Cells = row.Cells
				}
			case CT_FLOAT:
				if row.Cells[where.Left].(float64) >= right.(float64) {
					fRow.Cells = row.Cells
				}
			case CT_STRING:
				if row.Cells[where.Left].(string) >= right.(string) {
					fRow.Cells = row.Cells
				}
			}
		case "!=":
			switch colType {
			case CT_INTEGER:
				if row.Cells[where.Left].(int) != right.(int) {
					fRow.Cells = row.Cells
				}
			case CT_FLOAT:
				if row.Cells[where.Left].(float64) != right.(float64) {
					fRow.Cells = row.Cells
				}
			case CT_STRING:
				if row.Cells[where.Left].(string) != right.(string) {
					fRow.Cells = row.Cells
				}
			}
		}
		outRows = append(outRows, fRow)
	}
	return outRows, nil
}

func (self *SwarmDB) Query(u *SWARMDBUser, query *QueryOption) (rows []Row, err error) {
	switch query.Type {
	case "Select":
		rows, err := self.QuerySelect(u, query)
		if err != nil {
			return rows, err
		}
		if len(rows) == 0 {
			return rows, fmt.Errorf("select query came back empty")
		}
		return rows, err
	case "Insert":
		err = self.QueryInsert(u, query)
		return rows, err

	case "Update":
		err = self.QueryUpdate(u, query)
		return rows, err

	case "Delete":
		err = self.QueryDelete(u, query)
		return rows, err
	}
	return rows, nil
}

func (self *SwarmDB) Scan(u *SWARMDBUser, tableOwnerID string, tableName string, columnName string, ascending int) (rows []Row, err error) {
	tblKey := self.GetTableKey(tableOwnerID, tableName)
	tbl, ok := self.tables[tblKey]
	if !ok {
		return rows, fmt.Errorf("No such table to scan [%s] - [%s]", tableOwnerID, tableName)
	}
	rows, err = tbl.Scan(u, columnName, ascending)
	if err != nil {
		fmt.Printf("\nError doing table scan: [%s]", err)
		return rows, err
	}
	rows, err = tbl.assignRowColumnTypes(rows)
	if err != nil {
		fmt.Printf("\nError assigning column types to row values")
		return rows, err
	}

	fmt.Printf("swarmdb Scan finished ok: %+v\n", rows)
	return rows, nil

}

func (self *SwarmDB) GetTable(u *SWARMDBUser, tableOwnerID string, tableName string) (tbl *Table, err error) {
	if len(tableName) == 0 {
		return tbl, fmt.Errorf("Invalid table [%s]", tableName)
		//TODO: SWARMDBError
	}
	if len(tableOwnerID) == 0 {
		tableOwnerID = u.Address
	}
	fmt.Printf("\nGetting Table [%s] with the Owner [%s]", tableName, tableOwnerID)
	tblKey := self.GetTableKey(tableOwnerID, tableName)

	if tbl, ok := self.tables[tblKey]; ok {
		fmt.Printf("\ntable[%v] exists, it is: %+v\n", tblKey, tbl)
		fmt.Printf("\nprimary column name GetTable: %+v -> columns: %+v\n", tbl.columns, tbl.primaryColumnName)
		return tbl, nil
	} else {
		fmt.Printf("\ntable key isn't in self.tables! what is this path for?")
		//TODO: this should throw an error if the table is not created
	
		return tbl, &TableNotExistError{tableName: tableName, ownerID: tableOwnerID}

		/*
		tbl = self.NewTable(tableOwnerID, tableName, 1) //TODO: encrypted needed
		err = tbl.OpenTable(u)
		if err != nil {
			return tbl, &TableNotExistError{tableName: tableName, ownerID: tableOwnerID}
		}
		return tbl, nil
		*/
	}
}

//TODO: correct all errorhandling to swarmdb defaults
func (self *SwarmDB) SelectHandler(u *SWARMDBUser, data string) (resp string, err error) {

	fmt.Printf("\n\nin swarmdb SelectHandler with data ... %s\n", data)
	// var rerr *RequestFormatError
	d, err := parseData(data)
	if err != nil {
		fmt.Printf("problem: %s\n", err)
		return resp, err
	}

	tblKey := self.GetTableKey(d.TableOwner, d.Table)

	switch d.RequestType {
	case "CreateTable":
		if len(d.Table) == 0 || len(d.Columns) == 0 {
			return resp, fmt.Errorf(`ERR: empty table and column`)
		}
		//TODO: Upon further review, could make a NewTable and then call this from tbl. ---
		_, err := self.CreateTable(u, d.Table, d.Columns, d.Encrypted)
		if err != nil {
			return resp, err
		}
		return "ok", err
	case "Put":
		fmt.Printf("\nPut DATA: [%+v]", d)
		tbl, err := self.GetTable(u, d.TableOwner, d.Table)
		if err != nil {
			fmt.Printf("\nNO TABLE: [%s]", err.Error())
			return resp, err
			//TODO: SWARMDBErr
		}
		d.Rows, err = tbl.assignRowColumnTypes(d.Rows)
		if err != nil {
			return resp, err
		}
		//TODO: Will we handle Multi-row puts?
		err = tbl.Put(u, d.Rows[0].Cells)
		if err != nil {
			return resp, fmt.Errorf("\nError trying to 'Put' [%s] -- Err: %s", err)
		}
		return "ok", nil
	case "Get":
		if len(d.Key) == 0 {
			return resp, fmt.Errorf("Missing key in GET")
		}
		tbl, err := self.GetTable(u, d.TableOwner, d.Table)
		if err != nil {
			return resp, err
		}
		primaryColumnColumnType := tbl.columns[tbl.primaryColumnName].columnType
		convertedKey, err := convertJSONValueToKey(primaryColumnColumnType, d.Key)
		if err != nil {
			return resp, err
		}
		ret, err := tbl.Get(u, convertedKey)
		if err != nil {
			return resp, err
		}
		return string(ret), nil
	case "Insert":
		if len(d.Key) == 0 {
			//TODO: Missing Key Handling
			return resp, fmt.Errorf("Missing Key/Value")
		}
		tbl, err := self.GetTable(u, d.TableOwner, d.Table)
		if err != nil {
			return resp, err
		}
		err = tbl.Insert(u, d.Rows[0].Cells)
		if err != nil {
			return resp, err
		}
		return "ok", nil
	case "Delete":
		if len(d.Key) == 0 {
			return resp, fmt.Errorf("Missing key")
		}
		tbl, err := self.GetTable(u, d.TableOwner, d.Table)
		if err != nil {
			return resp, err
		}
		_, err = tbl.Delete(u, d.Key)
		if err != nil {
			return resp, err
		}
		return "ok", nil
		/* TODO:
		case "StartBuffer":
			err := tbl.StartBuffer()
			ret := "okay"
			if err != nil{
				ret = err.Error()
			}
			return ret
		case "FlushBuffer":
			err := tbl.FlushBuffer()
			ret := "okay"
			if err != nil{
				ret = err.Error()
			}
			return ret
		*/
	case "Query":
		fmt.Printf("\nReceived GETQUERY\n")
		if len(d.RawQuery) == 0 {
			return resp, fmt.Errorf("RawQuery is blank")
		}

		query, err := ParseQuery(d.RawQuery)
		if err != nil {
			fmt.Printf("err comes from query: [%s]\n", d.RawQuery)
			return resp, err
		}
		if len(d.Table) == 0 {
			fmt.Printf("Getting Table from Query rather than data obj\n")
			d.Table = query.Table //since table is specified in the query we do not have get it as a separate input
		}

		fmt.Printf("right before GetTable, u: %v, d.TableOwner: %v, d.Table: %v \n", u, d.TableOwner, d.Table)
		tbl, err := self.GetTable(u, d.TableOwner, d.Table)
		if err != nil {
			return resp, err
		}
		fmt.Printf("Returned table [%+v] when calling gettable with Owner[%s], Table[%s]\n", tbl, d.TableOwner, d.Table)
		tblInfo, err := tbl.GetTableInfo()
		if err != nil {
			fmt.Printf("tblInfo err \n")
			return resp, err
		}
		query.TableOwner = d.TableOwner //probably should check the owner against the tableinfo owner here

		fmt.Printf("Table info gotten: [%+v] \n", tblInfo)
		fmt.Printf("QueryOption is: [%+v] \n", query)

		/*
			fmt.Printf("The other way of getting tableinfo\n")
			tblKey := self.GetTableKey(d.TableOwner, d.Table)
			tblInfo, err := self.tables[tblKey].GetTableInfo()
			if err != nil {
			        return resp, err
			}
		*/

		//checking validity of columns
		for _, reqCol := range query.RequestColumns {
			if _, ok := tblInfo[reqCol.ColumnName]; !ok {
				return resp, fmt.Errorf("Requested col [%s] does not exist in table [%+v]\n", reqCol.ColumnName, tblInfo)
			}
		}

		//checking the Where clause
		if len(query.Where.Left) > 0 {
			if _, ok := tblInfo[query.Where.Left]; !ok {
				return resp, fmt.Errorf("Query col [%s] does not exist in table\n", query.Where.Left)
			}

			//checking if the query is just a primary key Get
			if query.Where.Left == tbl.primaryColumnName && query.Where.Operator == "=" {
				fmt.Printf("Calling Get from Query\n")

				convertedKey, err := convertJSONValueToKey(tbl.columns[tbl.primaryColumnName].columnType, query.Where.Right)
				if err != nil {
					//TODO: ConvertingJSONToKey Error
					return resp, err
				}

				byteRow, err := tbl.Get(u, convertedKey)
				if err != nil {
					fmt.Printf("Error Calling Get from Query [%s]\n", err)
					return resp, err
				}

				row, err := tbl.byteArrayToRow(byteRow)
				fmt.Printf("Response row from Get: %s (%v)\n", row, row)
				if err != nil {
					return resp, err
				}

				filteredRow := filterRowByColumns(&row, query.RequestColumns)
				fmt.Printf("\nResponse filteredrow from Get: %s (%v)", filteredRow.Cells, filteredRow.Cells)
				retJson, err := json.Marshal(filteredRow.Cells)
				if err != nil {
					return resp, err
				}

				return string(retJson), nil
			}
		}
		fmt.Printf("\nAbout to process query [%+v]", query)
		//process the query
		qRows, err := self.Query(u, &query)
		fmt.Printf("\nQRows: [%+v]", qRows)
		if err != nil {
			fmt.Printf("\nError processing query [%+v] | Error: %s", query, err)
			return resp, err
		}
		resp, err = rowDataToJson(qRows)
		fmt.Printf("\nJSONED Row is: [%+v] [%s]", resp, resp)
		if err != nil {
			return resp, err
		}
		return resp, nil

	case "GetTableInfo":
		tblcols, err := self.tables[tblKey].GetTableInfo()
		if err != nil {
			return resp, err
		}
		tblinfo, err := json.Marshal(tblcols)
		if err != nil {
			return resp, err
		}
		return string(tblinfo), nil
	}
	return resp, fmt.Errorf("RequestType invalid: [%s]", d.RequestType)
}

func parseData(data string) (*RequestOption, error) {
	udata := new(RequestOption)
	if err := json.Unmarshal([]byte(data), udata); err != nil {
		fmt.Printf("BIG PROBLEM parsing [%s] | Error: %v\n", data, err)
		return nil, err
	}
	return udata, nil
}

func (t *Table) Scan(u *SWARMDBUser, columnName string, ascending int) (rows []Row, err error) {
	column, err := t.getColumn(columnName)
	if t.primaryColumnName != columnName {
		fmt.Printf("\nSkipping column [%s]", columnName)
		return rows, err
	}
	if err != nil {
		fmt.Printf("table Scan getColumn err %v \n", err)
		return rows, err
	}
	c := column.dbaccess.(OrderedDatabase)
	// TODO: Error checking

	fmt.Printf("\nProcessing column [%s]", columnName)
	if ascending == 1 {
		res, err := c.SeekFirst(u)
		if err != nil {
			fmt.Printf("\nError in table.Scan: ", err)
		} else {
			records := 0
			for k, v, err := res.Next(u); err == nil; k, v, err = res.Next(u) {
				fmt.Printf("\n *int*> %d: K: %s V: %v (%s) \n", records, KeyToString(column.columnType, k), v, v)
				row, _ := t.Get(u, k)
				//TODO: GetError
				rowObj, _ := t.byteArrayToRow(row)
				//TODO: ArrayToRowConversionError
				if err != nil {
					fmt.Printf("\nError converting v => [%s] bytearray to row: [%s]", v, err)
					return rows, err
				}
				fmt.Printf("table Scan, row set: %+v\n", row)
				rows = append(rows, rowObj)
				records++
			}
		}
	} else {
		res, err := c.SeekLast(u)
		if err != nil {
			fmt.Printf("\nError in table.Scan: ", err)
		} else {
			records := 0
			for k, v, err := res.Prev(u); err == nil; k, v, err = res.Prev(u) {
				fmt.Printf(" *int*> %d: K: %s V: %v\n", records, KeyToString(CT_STRING, k), KeyToString(column.columnType, v))
				row, err := t.byteArrayToRow(v)
				if err != nil {
					fmt.Printf("table Scan, byteArrayToRow err: %+v\n", err)
					return rows, err
				}
				fmt.Printf("table Scan, row set: %+v\n", row)
				rows = append(rows, row)
				records++
			}
		}
	}
	fmt.Printf("table Scan, rows returned: %+v\n", rows)
	return rows, nil
}

func (self *SwarmDB) NewTable(ownerID string, tableName string, encrypted int) *Table {
	t := new(Table)
	t.swarmdb = self
	t.ownerID = ownerID
	t.tableName = tableName
	t.encrypted = encrypted
	t.columns = make(map[string]*ColumnInfo)

	// register the Table in SwarmDB
	tblKey := self.GetTableKey(ownerID, tableName)
	self.tables[tblKey] = t
	return t
}

//TODO: need to make sure the types of the columns are correct
func (swdb *SwarmDB) CreateTable(u *SWARMDBUser, tableName string, columns []Column, encrypted int) (tbl *Table, err error) {
	columnsMax := 30
	primaryColumnName := ""
	if len(columns) > columnsMax {
		fmt.Printf("\nMax Allowed Columns for a table is %s and you submit %s", columnsMax, len(columns))
	}

	//error checking
	for _, columninfo := range columns {
		if columninfo.Primary > 0 {
			if len(primaryColumnName) > 0 {
				return tbl, fmt.Errorf("more than one primary column")
			}
			primaryColumnName = columninfo.ColumnName
		}
		if !CheckColumnType(columninfo.ColumnType) {
			return tbl, fmt.Errorf("bad columntype")
		}
		if !CheckIndexType(columninfo.IndexType) {
			return tbl, fmt.Errorf("bad column indextype")
		}
	}
	if len(primaryColumnName) == 0 {
		return tbl, fmt.Errorf("no primary column indicated")
		//TODO: SWARMDBError
	}

	buf := make([]byte, 4096)
	fmt.Printf("\nCreating Table [%s] with the Owner [%s]", tableName, u.Address)
	tbl = swdb.NewTable(u.Address, tableName, encrypted)
	for i, columninfo := range columns {
		copy(buf[2048+i*64:], columninfo.ColumnName)
		b := make([]byte, 1)
		b[0] = byte(columninfo.Primary)
		copy(buf[2048+i*64+26:], b)

		b[0] = byte(columninfo.ColumnType)
		copy(buf[2048+i*64+28:], b)

		b[0] = byte(columninfo.IndexType)
		copy(buf[2048+i*64+30:], b) // columninfo.IndexType
		// fmt.Printf(" column: %v\n", columninfo)
	}

	//Could (Should?) be less bytes, but leaving space in case more is to be there
	copy(buf[4000:4024], IntToByte(tbl.encrypted))
	swarmhash, err := swdb.StoreDBChunk(u, buf, tbl.encrypted) // TODO
	if err != nil {
		fmt.Printf(" problem storing chunk\n")
		return tbl, err
	}
	tbl.primaryColumnName = primaryColumnName
	//tbl.tableName = tableName //Redundant? - because already set in NewTable?

	fmt.Printf(" CreateTable primary: [%s] (%s) store root hash:  %s vs %s hash:[%x]\n", tbl.primaryColumnName, tbl.ownerID, tableName, tbl.tableName, swarmhash)
	err = swdb.StoreRootHash([]byte(tbl.tableName), []byte(swarmhash))
	if err != nil {
		return tbl, err
		//TODO: SWARMDBError
	}
	err = tbl.OpenTable(u)
	if err != nil {
		return tbl, err
	}
	return tbl, nil
}

func (t *Table) OpenTable(u *SWARMDBUser) (err error) {
	t.swarmdb.Logger.Debug(fmt.Sprintf("swarmdb.go:OpenTable|%s", t.tableName))
	t.columns = make(map[string]*ColumnInfo)

	/// get Table RootHash to  retrieve the table descriptor
	roothash, err := t.swarmdb.GetRootHash([]byte(t.tableName))
	fmt.Printf("opening table @ %s roothash %s\n", t.tableName, roothash)
	if err != nil {
		err = fmt.Errorf("Error retrieving Index Root Hash for table [%s]: %v", t.tableName, err)
		return err
	}
	if len(roothash) == 0 {
		err = fmt.Errorf("Empty hash retrieved, %v", err)
		return err
	}
	setprimary := false
	columndata, err := t.swarmdb.RetrieveDBChunk(u, roothash)
	if err != nil {
		err = fmt.Errorf("Error retrieving Index Root Hash: %v", err)
		return err
	}

	columnbuf := columndata
	primaryColumnType := ColumnType(CT_INTEGER)
	for i := 2048; i < 4000; i = i + 64 {
		buf := make([]byte, 64)
		copy(buf, columnbuf[i:i+64])
		if buf[0] == 0 {
			fmt.Printf("\nin swarmdb.OpenTable, skip!\n")
			break
		}
		columninfo := new(ColumnInfo)
		columninfo.columnName = string(bytes.Trim(buf[:25], "\x00"))
		columninfo.primary = uint8(buf[26])
		columninfo.columnType = ColumnType(buf[28]) //:29
		columninfo.indexType = IndexType(buf[30])
		columninfo.roothash = buf[32:]
		secondary := false
		if columninfo.primary == 0 {
			secondary = true
		} else {
			primaryColumnType = columninfo.columnType // TODO: what if primary is stored *after* the secondary?  would break this..
		}
		fmt.Printf("\n columnName: %s (%d) roothash: %x (secondary: %v) columnType: %d", columninfo.columnName, columninfo.primary, columninfo.roothash, secondary, columninfo.columnType)
		switch columninfo.indexType {
		case IT_BPLUSTREE:
			//need to add in err for NewBPlusTreeDB
			bplustree := NewBPlusTreeDB(u, *t.swarmdb, columninfo.roothash, ColumnType(columninfo.columnType), secondary, ColumnType(primaryColumnType))
			// bplustree.Print()
			columninfo.dbaccess = bplustree
			if err != nil { //this should be the err for NewBPlusTreeDB
				return err
			}
		case IT_HASHTREE:
			columninfo.dbaccess, err = NewHashDB(u, columninfo.roothash, *t.swarmdb, ColumnType(columninfo.columnType))
			if err != nil {
				return err
			}
		}
		t.columns[columninfo.columnName] = columninfo
		if columninfo.primary == 1 {
			if !setprimary {
				t.primaryColumnName = columninfo.columnName
			} else {
				var rerr *RequestFormatError
				return rerr
			}
		}
	}
	//Redundant? -- t.encrypted = BytesToInt64(columnbuf[4000:4024])
	return nil
}

func convertJSONValueToKey(columnType ColumnType, pvalue interface{}) (k []byte, err error) {
	switch svalue := pvalue.(type) {
	case (int):
		i := fmt.Sprintf("%d", svalue)
		k = StringToKey(columnType, i)
	case (float64):
		f := ""
		switch columnType {
		case CT_INTEGER:
			f = fmt.Sprintf("%d", int(svalue))
		case CT_FLOAT:
			f = fmt.Sprintf("%f", svalue)
		case CT_STRING:
			f = fmt.Sprintf("%f", svalue)
		}
		k = StringToKey(columnType, f)
	case (string):
		k = StringToKey(columnType, svalue)
	default:
		return k, fmt.Errorf("Unknown Type: %v\n", reflect.TypeOf(svalue))
	}
	return k, nil
}

func convertMapValuesToStrings(in map[string]interface{}) (map[string]string, error) {
	out := make(map[string]string)
	var err error
	for key, value := range in {
		switch value := value.(type) {
		case int:
			out[key] = strconv.Itoa(value)
		case int64:
			out[key] = strconv.FormatInt(value, 10)
		case float64:
			out[key] = strconv.FormatFloat(value, 'f', -1, 64)
		case string:
			out[key] = value
		default:
			err = fmt.Errorf("value %v has unknown type", value)
		}
	}
	return out, err
}

func (t *Table) Put(u *SWARMDBUser, row map[string]interface{}) (err error) {

	rawvalue, err := json.Marshal(row)
	if err != nil {
		return err
	}
	t.swarmdb.Logger.Debug(fmt.Sprintf("swarmdb.go:Put|%s", rawvalue))
	k := make([]byte, 32)

	for _, c := range t.columns {
		//fmt.Printf("\nProcessing a column %s and primary is %d", c.columnName, c.primary)
		if c.primary > 0 {

			pvalue, ok := row[t.primaryColumnName]
			if !ok {
				return fmt.Errorf("\nPrimary key %s not specified in input", t.primaryColumnName)
			}
			k, err = convertJSONValueToKey(t.columns[t.primaryColumnName].columnType, pvalue)
			if err != nil {
				return err
			}

			t.swarmdb.kaddb.Open([]byte(t.ownerID), []byte(t.tableName), []byte(t.primaryColumnName), t.encrypted)
			khash, err := t.swarmdb.kaddb.Put(u, k, []byte(rawvalue)) // TODO -- use u (sk)
			if err != nil {
				fmt.Printf("\nKademlia Put Failed")
				return err
			}
			// fmt.Printf(" - primary  %s | %x\n", c.columnName, k)
			_, err = t.columns[c.columnName].dbaccess.Put(u, k, khash) // TODO: Check Error for bplus/hashdb put
			//			t.columns[c.columnName].dbaccess.Print()
			if err != nil {
				return err
			}
		} else {
			k2 := make([]byte, 32)
			var errPvalue error
			pvalue, ok := row[c.columnName]
			if !ok {
				//this is ok <- WHY? TODO
				//return fmt.Errorf("Column [%s] not found in [%+v]", c.columnName, jsonrecord)
			}
			k2, errPvalue = convertJSONValueToKey(c.columnType, pvalue)
			if errPvalue != nil {
				fmt.Printf("\nERROR: [%s]", errPvalue)
				return err
			}

			fmt.Printf(" - secondary %s %x | %x\n", c.columnName, k2, k)
			_, err = t.columns[c.columnName].dbaccess.Put(u, k2, k)
			if err != nil {
				fmt.Errorf("\nDB Put Failed")
				return err
			}

			//t.columns[c.columnName].dbaccess.Print()
		}
	}

	if t.buffered {
		//TODO: is something supposed to be here?
	} else {
		err = t.FlushBuffer(u)
		if err != nil {
			fmt.Printf("flushing err %v\n")
		} else {
			//TODO: is something supposed to be here?
		}
	}
	/*
		switch x := t.columns[t.primaryColumnName].dbaccess.(type) {
		case (*Tree):
			fmt.Printf("B+ tree Print (%s)\n", value)
			x.Print()
			fmt.Printf("-------\n\n")
		}
	*/

	return nil
}

//TODO: this is commented out because this insert: t.columns[primaryColumnName].dbaccess.Insert(k, []byte(khash))  doesn't have anything
func (t *Table) Insert(u *SWARMDBUser, row map[string]interface{}) (err error) {
	//TODO: Delete this?
	/*
		 value, err := convertMapValuesToStrings(row)
		 if err != nil {
		    return err
		 }
				t.swarmdb.Logger.Debug(fmt.Sprintf("swarmdb.go:Insert|%s", value))
				primaryColumnName := t.primaryColumnName
				/// store value to kdb and get a hash
				_, b, err := t.columns[primaryColumnName].dbaccess.Get([]byte(key))
				if b {
					var derr *DuplicateKeyError
					return derr
				}
				if err != nil {
					return err
				}

				t.swarmdb.kaddb.Open([]byte(t.ownerID), []byte(t.tableName), []byte(primaryColumnName), t.encrypted)
				k := StringToKey(t.columns[primaryColumnName].columnType, key)
				khash, err := t.swarmdb.kaddb.Put(k, []byte(value))
				if err != nil {
					return err
				}
				_, err = t.columns[primaryColumnName].dbaccess.Insert(k, []byte(khash))
	*/
	return err
}

func (t *Table) getPrimaryColumn() (c *ColumnInfo, err error) {
	return t.getColumn(t.primaryColumnName)
}

func (t *Table) getColumn(columnName string) (c *ColumnInfo, err error) {
	if t.columns[columnName] == nil {
		//var cerr *NoColumnError
		cerr := &NoColumnError{tableName: t.tableName, tableOwner: t.ownerID, columnName: columnName}
		return c, cerr
	}
	return t.columns[columnName], nil
}

func (t *Table) byteArrayToRow(byteData []byte) (out Row, err error) {
	fmt.Printf("\nConverting bd [%s][%v]", byteData, byteData)
	row := NewRow()
	//row.primaryKeyValue = t.primaryColumnName
	if err := json.Unmarshal(byteData, &row.Cells); err != nil {
		fmt.Printf("\nReturning row [%+v] for bytedata[%v]", row, byteData)
		return row, err
	}
	return row, nil
}

func (t *Table) Get(u *SWARMDBUser, key []byte) (out []byte, err error) {
	t.swarmdb.Logger.Debug(fmt.Sprintf("swarmdb.go:Get|%s", key))
	primaryColumnName := t.primaryColumnName
	if t.columns[primaryColumnName] == nil {
		fmt.Printf("NO COLUMN ERROR\n")
		var cerr *NoColumnError
		return nil, cerr
	}
	// fmt.Printf("READY\n")

	t.swarmdb.kaddb.Open([]byte(t.ownerID), []byte(t.tableName), []byte(t.primaryColumnName), t.encrypted)
	fmt.Printf("\n GET key: (%s)%v\n", key, key)

	v, ok, err2 := t.columns[primaryColumnName].dbaccess.Get(u, key)
	//TODO: handle "ok"s
	fmt.Printf("\n v retrieved from db traversal get = %s", v)
	if err2 != nil {
		fmt.Printf("\nError traversing tree: %s", err.Error())
		return nil, err2
	}
	if ok {
		// get value from kdb
		kres, err3 := t.swarmdb.kaddb.GetByKey(u, key)
		if err3 != nil {
			return out, err3
		}
		fres := bytes.Trim(kres, "\x00")
		return fres, nil
	}
	fmt.Printf("\n MISSING RECORD %s\n", key)
	return []byte(""), nil

}

func (t *Table) Delete(u *SWARMDBUser, key string) (ok bool, err error) {
	t.swarmdb.Logger.Debug(fmt.Sprintf("swarmdb.go:Delete|%s", key))
	primaryColumnName := t.primaryColumnName
	k := StringToKey(t.columns[primaryColumnName].columnType, key)
	ok = false
	for _, ip := range t.columns {
		ok2, err := ip.dbaccess.Delete(u, k)
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			return ok2, err
		}
		if ok2 {
			ok = true
		}
	}
	return ok, nil
}

func (t *Table) StartBuffer(u *SWARMDBUser) (err error) {
	t.swarmdb.Logger.Debug(fmt.Sprintf("swarmdb.go:StartBuffer|%s", t.primaryColumnName))
	if t.buffered {
		t.FlushBuffer(u)
	} else {
		t.buffered = true
	}

	for _, ip := range t.columns {
		_, err := ip.dbaccess.StartBuffer(u)
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *Table) FlushBuffer(u *SWARMDBUser) (err error) {
	t.swarmdb.Logger.Debug(fmt.Sprintf("swarmdb.go:FlushBuffer|%s", t.primaryColumnName))

	for _, ip := range t.columns {
		_, err := ip.dbaccess.FlushBuffer(u)
		if err != nil {
			fmt.Printf(" ERR1 %v\n", err)
			return err
		}
		roothash, err := ip.dbaccess.GetRootHash()
		ip.roothash = roothash
	}
	err = t.updateTableInfo(u)
	if err != nil {
		fmt.Printf(" err %v \n", err)
		return err
	}
	return nil
}

func (t *Table) updateTableInfo(u *SWARMDBUser) (err error) {
	buf := make([]byte, 4096)
	i := 0
	for column_num, c := range t.columns {
		b := make([]byte, 1)

		copy(buf[2048+i*64:], column_num)

		b[0] = byte(c.primary)
		copy(buf[2048+i*64+26:], b)

		b[0] = byte(c.columnType)
		copy(buf[2048+i*64+28:], b)

		b[0] = byte(c.indexType)
		copy(buf[2048+i*64+30:], b)

		copy(buf[2048+i*64+32:], c.roothash)
		i++
	}
	isEncrypted := 1
	swarmhash, err := t.swarmdb.StoreDBChunk(u, buf, isEncrypted)
	if err != nil {
		return err
	}
	err = t.swarmdb.StoreRootHash([]byte(t.tableName), []byte(swarmhash))
	// fmt.Printf(" STORE ROOT HASH [%s] ==> %x\n", t.tableName, swarmhash)
	if err != nil {
		fmt.Printf("StoreRootHash ERROR %v\n", err)
		return err
	} else {
	}
	return nil
}

func (swdb *SwarmDB) GetTableKey(owner string, tableName string) (key string) {
	return (fmt.Sprintf("%s|%s", owner, tableName))
}

func (t *Table) GetTableInfo() (tblInfo map[string]Column, err error) {
	//var columns []Column
	fmt.Printf("\n in GetTableInfo with table [%+v] \n", t)
	tblInfo = make(map[string]Column)
	for cname, c := range t.columns {
		var cinfo Column
		cinfo.ColumnName = cname
		cinfo.IndexType = c.indexType
		cinfo.Primary = int(c.primary)
		cinfo.ColumnType = c.columnType
		//	fmt.Printf("\nProcessing columng [%s]", cname)
		if _, ok := tblInfo[cname]; ok { //would mean for some reason there are two cols named the same thing
			fmt.Printf("\nERROR: Duplicate column? [%s]", cname)
			return tblInfo, err
		}
		tblInfo[cname] = cinfo
		//columns = append(columns, cinfo)
	}
	//jcolumns, err := json.Marshal(columns)

	//return string(jcolumns), err
	return tblInfo, err
}
