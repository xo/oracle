package types

// TypeID is the number by which the protocol names an Oracle type, as the
// type of a column or of a bind.
type TypeID uint16

// The Oracle type IDs.
const (
	NCHAR                 TypeID = 1
	NUMBER                TypeID = 2
	BInteger              TypeID = 3
	FLOAT                 TypeID = 4
	NullStr               TypeID = 5
	VarNum                TypeID = 6
	PDN                   TypeID = 7
	LONG                  TypeID = 8
	VARCHAR               TypeID = 9
	ROWID                 TypeID = 11
	DATE                  TypeID = 12
	VarRaw                TypeID = 15
	BFLOAT                TypeID = 21
	BDOUBLE               TypeID = 22
	RAW                   TypeID = 23
	LongRaw               TypeID = 24
	TNSJSONTypeDate       TypeID = 60
	TNSJSONTypeIntervalYM TypeID = 61
	TNSJSONTypeIntervalDS TypeID = 62
	UINT                  TypeID = 68
	LongVarChar           TypeID = 94
	LongVarRaw            TypeID = 95
	CHAR                  TypeID = 96
	CHARZ                 TypeID = 97
	IBFLOAT               TypeID = 100
	IBDOUBLE              TypeID = 101
	REFCURSOR             TypeID = 102
	OCIXMLType            TypeID = 108
	XMLType               TypeID = 109
	OCIRef                TypeID = 110
	OCIClobLocator        TypeID = 112
	OCIBlobLocator        TypeID = 113
	OCIFileLocator        TypeID = 114
	RESULTSET             TypeID = 116
	JSONType              TypeID = 119
	TNSDataTypeOAC122     TypeID = 120
	VECTOR                TypeID = 127
	OCIString             TypeID = 155
	OCIDate               TypeID = 156
	TimestampDTY          TypeID = 180
	TimestampTZDTY        TypeID = 181
	IntervalYMDTY         TypeID = 182
	IntervalDSDTY         TypeID = 183
	TimeTZ                TypeID = 186
	TIMESTAMP             TypeID = 187
	TIMESTAMPTZ           TypeID = 188
	INTERVALYM            TypeID = 189
	INTERVALDS            TypeID = 190
	UROWID                TypeID = 208
	TimestampLTZDTY       TypeID = 231
	TimestampLTZ          TypeID = 232
	BOOLEAN               TypeID = 0xFC
)
