package utils

import (
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

const SheetName = "Sheet1"

// ExcelColumn describes one column of an import/export/template spec for a table.
// Get extracts a display string from a loaded model instance for export.
// Set parses a raw cell string and applies it onto a pointer to a new model instance for import.
type ExcelColumn struct {
	Header   string
	Required bool
	Example  string
	Get      func(item interface{}) string
	Set      func(item interface{}, raw string) error
}

func headerStyle(f *excelize.File) int {
	style, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"2563EB"}, Pattern: 1},
		Alignment: &excelize.Alignment{
			Horizontal: "center",
			Vertical:   "center",
		},
	})
	return style
}

func writeHeader(f *excelize.File, columns []ExcelColumn) {
	for i, col := range columns {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(SheetName, cell, col.Header)
	}
	if len(columns) > 0 {
		lastCol, _ := excelize.CoordinatesToCellName(len(columns), 1)
		_ = f.SetCellStyle(SheetName, "A1", lastCol, headerStyle(f))
		for i := range columns {
			colName, _ := excelize.ColumnNumberToName(i + 1)
			_ = f.SetColWidth(SheetName, colName, colName, 24)
		}
	}
}

// GenerateTemplate builds a downloadable .xlsx template: header row + one example row.
func GenerateTemplate(columns []ExcelColumn) *excelize.File {
	f := excelize.NewFile()
	f.SetSheetName("Sheet1", SheetName)
	writeHeader(f, columns)
	for i, col := range columns {
		if col.Example != "" {
			cell, _ := excelize.CoordinatesToCellName(i+1, 2)
			_ = f.SetCellValue(SheetName, cell, col.Example)
		}
	}
	return f
}

// ExportData writes a slice of model items (pass a []T as interface{}) into an .xlsx file.
func ExportData(items interface{}, columns []ExcelColumn) (*excelize.File, error) {
	v := reflect.ValueOf(items)
	if v.Kind() != reflect.Slice {
		return nil, fmt.Errorf("data yang diexport harus berupa slice")
	}
	f := excelize.NewFile()
	f.SetSheetName("Sheet1", SheetName)
	writeHeader(f, columns)
	for r := 0; r < v.Len(); r++ {
		item := v.Index(r).Interface()
		for c, col := range columns {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			var val string
			if col.Get != nil {
				val = col.Get(item)
			}
			_ = f.SetCellValue(SheetName, cell, val)
		}
	}
	return f, nil
}

// ReadRows opens an uploaded .xlsx and returns all data rows (header row excluded).
func ReadRows(r io.Reader) ([][]string, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, fmt.Errorf("file bukan format excel (.xlsx) yang valid")
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("file excel tidak memiliki sheet")
	}
	sheetName := SheetName
	found := false
	for _, s := range sheets {
		if s == SheetName {
			found = true
			break
		}
	}
	if !found {
		sheetName = sheets[0]
	}

	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, err
	}
	if len(rows) <= 1 {
		return [][]string{}, nil
	}

	// Excel files often report far more rows than actually contain data (leftover
	// formatting/styling on "empty" rows below the real data). Skip any row whose
	// cells are all blank so those don't get treated as invalid data rows.
	var result [][]string
	for _, row := range rows[1:] {
		blank := true
		for _, cell := range row {
			if strings.TrimSpace(cell) != "" {
				blank = false
				break
			}
		}
		if !blank {
			result = append(result, row)
		}
	}
	return result, nil
}

// ImportRow applies one row of raw cell strings onto newItem (a pointer to a struct)
// using the column Set functions. Returns a list of human-readable validation errors.
func ImportRow(newItem interface{}, row []string, columns []ExcelColumn) []string {
	var errs []string
	for i, col := range columns {
		var raw string
		if i < len(row) {
			raw = strings.TrimSpace(row[i])
		}
		if col.Required && raw == "" {
			errs = append(errs, fmt.Sprintf("%s wajib diisi", col.Header))
			continue
		}
		if raw == "" {
			continue
		}
		if col.Set != nil {
			if err := col.Set(newItem, raw); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", col.Header, err))
			}
		}
	}
	return errs
}

// ---- small parsing helpers reused by per-entity column Set functions ----

func ParseDateCell(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	layouts := []string{"2006-01-02", "02/01/2006", "02-01-2006", "2006/01/02"}
	for _, l := range layouts {
		if t, err := time.Parse(l, raw); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("format tanggal tidak valid, gunakan YYYY-MM-DD (nilai: %s)", raw)
}

func ParseIntCell(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	// tolerate values like "12.0" coming from excel numeric cells
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		return int(f), nil
	}
	return strconv.Atoi(raw)
}

func FormatDateCell(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}
