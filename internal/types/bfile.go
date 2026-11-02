package types

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/xo/oracle/internal/charset"
)

type BFile struct {
	Basic
	lobBase
	//dirName  string
	//fileName string
	Conv     *charset.Converter
	isOpened bool
}

// CreateBFile returns a BFILE for the file fileName in the Oracle directory
// dirName, with a streamer from db.
func CreateBFile(ctx context.Context, db *sql.DB, dirName, fileName string) (*BFile, error) {
	var err error
	ret := &BFile{}
	err = ret.createStreamer(ctx, db)
	if err != nil {
		return nil, err
	}
	// the names are in the character set of the database. go-ora wrote them
	// in AL16UTF16, and the server found no file for the BFILE
	ret.Conv, err = ret.stream.StringCoder().DefaultStringCoder()
	if err != nil {
		return nil, err
	}
	filePath := ""
	if len(dirName) != 0 && len(fileName) != 0 {
		filePath = dirName + "/" + fileName
	}
	return ret, ret.SetValue(filePath)
}
func (file *BFile) MaxLen() int64 {
	return MaxLenBFile
}
func (file *BFile) SetValue(input any) error {
	if input == nil {
		file.bValue = nil
		return nil
	}
	var fileName, dirName []byte
	if file.Conv == nil {
		file.Conv = charset.New(0x7D0)
	}
	switch input := input.(type) {
	case BFile:
		*file = input
		return nil
	case *BFile:
		*file = *input
		return nil
	case string:
		if len(input) == 0 {
			file.bValue = nil
			return nil
		}
		index := strings.Index(input, "\\")
		if index < 0 {
			index = strings.Index(input, "/")
			if index < 0 {
				return fmt.Errorf("invalid file path: %s (should be dirname/filename)", input)
			}
		}
		dirName = file.Conv.Encode(input[:index])
		fileName = file.Conv.Encode(input[index+1:])
	case *string:
		if len(*input) == 0 {
			file.bValue = nil
			return nil
		}
		index := strings.Index(*input, "\\")
		if index < 0 {
			index = strings.Index(*input, "/")
			if index < 0 {
				return fmt.Errorf("invalid file path: %s (should be dirname/filename)", *input)
			}
		}
		dirName = file.Conv.Encode((*input)[:index])
		fileName = file.Conv.Encode((*input)[index+1:])
	default:
		return fmt.Errorf("cannot set value of type: %T into BFile", input)
	}
	totalLen := 16 + len(dirName) + len(fileName) + 4
	locatorBuffer := &bytes.Buffer{}
	err := binary.Write(locatorBuffer, binary.BigEndian, uint16(totalLen-2))
	if err != nil {
		return err
	}
	locatorBuffer.Write([]byte{0, 1, 8, 8, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0})
	err = binary.Write(locatorBuffer, binary.BigEndian, uint16(len(dirName)))
	if err != nil {
		return err
	}
	if len(dirName) > 0 {
		locatorBuffer.Write(dirName)
	}
	err = binary.Write(locatorBuffer, binary.BigEndian, uint16(len(fileName)))
	if err != nil {
		return err
	}
	if len(fileName) > 0 {
		locatorBuffer.Write(fileName)
	}
	file.bValue = locatorBuffer.Bytes()
	if file.stream != nil {
		file.stream.SetLocator(file.bValue)
	}
	return nil
}

func (file *BFile) SetStreamer(input LobStreamer) {
	file.stream = input
	if input != nil {
		// get dir and file name before change conversion
		dirName := file.DirName()
		fileName := file.FileName()
		file.Conv, _ = file.stream.StringCoder().DefaultStringCoder()
		filePath := ""
		if len(dirName) != 0 && len(fileName) != 0 {
			filePath = dirName + "/" + fileName
		}
		_ = file.SetValue(filePath)
	}
}
func (file *BFile) Value() (any, error) {
	return file.bValue, nil
}

func (file *BFile) Scan(input any) error {
	return file.SetValue(input)
}

func (file *BFile) IsOpen() bool {
	return file.isOpened
}

// IsInit reports whether the BFile has a locator and a streamer to read it
// with. A BFile of a row holds its locator in its streamer alone, which
// go-ora did not look at, so it was never initialized and did not open.
func (file *BFile) IsInit() bool {
	if file.stream == nil {
		return false
	}
	if file.stream.Locator() == nil && file.bValue != nil {
		file.stream.SetLocator(file.bValue)
	}
	return len(file.stream.Locator()) > 0
}

func (file *BFile) Open(ctx context.Context) error {
	if !file.IsInit() {
		return errors.New("BFile is not initialized")
	}
	if file.isOpened {
		return nil
	}
	err := file.stream.Open(0xB, 0x100)
	if err != nil {
		return err
	}
	file.isOpened = true
	return nil
}

func (file *BFile) Close() error {
	if !file.IsInit() {
		return errors.New("BFile is not initialized")
	}
	if !file.isOpened {
		return nil
	}
	err := file.stream.Close(0x200)
	if err != nil {
		return err
	}
	file.isOpened = false
	return nil
}

func (file *BFile) DirName() string {
	dirName := ""
	locator := file.Locator()
	if len(locator) > 16 && file.Conv != nil {
		index := 16
		length := int(binary.BigEndian.Uint16(locator[index : index+2]))
		index += 2
		dirName = file.Conv.Decode(locator[index : index+length])
	}
	return dirName
}

func (file *BFile) FileName() string {
	fileName := ""
	locator := file.Locator()
	if len(locator) > 16 && file.Conv != nil {
		index := 16
		length := int(binary.BigEndian.Uint16(locator[index : index+2]))
		index += 2
		_ = file.Conv.Decode(locator[index : index+length])
		index += length
		length = int(binary.BigEndian.Uint16(locator[index : index+2]))
		index += 2
		fileName = file.Conv.Decode(locator[index : index+length])
	}
	return fileName
}

// Exists reports whether the file of the BFile exists, open or not, as the
// fileexists of python-oracledb does. go-ora asked for an open file, and a
// file that does not exist cannot open.
func (file *BFile) Exists() (bool, error) {
	if !file.IsInit() {
		return false, errors.New("BFile is not initialized")
	}
	return file.stream.Exists()
}
