// Copyright 2016 - 2023 The excelize Authors. All rights reserved. Use of
// this source code is governed by a BSD-style license that can be found in
// the LICENSE file.
//
// Package excelize providing a set of functions that allow you to write to and
// read from XLAM / XLSM / XLSX / XLTM / XLTX files. Supports reading and
// writing spreadsheet documents generated by Microsoft Excel™ 2007 and later.
// Supports complex components by high compatibility, and provided streaming
// API for generating or reading data from a worksheet with huge amounts of
// data. This library needs Go version 1.16 or later.

package excelize

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"

	"github.com/xuri/efp"
)

type adjustDirection bool

const (
	columns adjustDirection = false
	rows    adjustDirection = true
)

// adjustHelper provides a function to adjust rows and columns dimensions,
// hyperlinks, merged cells and auto filter when inserting or deleting rows or
// columns.
//
// sheet: Worksheet name that we're editing
// column: Index number of the column we're inserting/deleting before
// row: Index number of the row we're inserting/deleting before
// offset: Number of rows/column to insert/delete negative values indicate deletion
//
// TODO: adjustPageBreaks, adjustComments, adjustDataValidations, adjustProtectedCells
func (f *File) adjustHelper(sheet string, dir adjustDirection, num, offset int) error {
	ws, err := f.workSheetReader(sheet)
	if err != nil {
		return err
	}
	sheetID := f.getSheetID(sheet)
	if dir == rows {
		err = f.adjustRowDimensions(sheet, ws, num, offset)
	} else {
		err = f.adjustColDimensions(sheet, ws, num, offset)
	}
	if err != nil {
		return err
	}
	f.adjustHyperlinks(ws, sheet, dir, num, offset)
	f.adjustTable(ws, sheet, dir, num, offset)
	if err = f.adjustMergeCells(ws, dir, num, offset); err != nil {
		return err
	}
	if err = f.adjustAutoFilter(ws, dir, num, offset); err != nil {
		return err
	}
	if err = f.adjustCalcChain(dir, num, offset, sheetID); err != nil {
		return err
	}
	ws.checkSheet()
	_ = ws.checkRow()

	if ws.MergeCells != nil && len(ws.MergeCells.Cells) == 0 {
		ws.MergeCells = nil
	}

	return nil
}

// adjustCols provides a function to update column style when inserting or
// deleting columns.
func (f *File) adjustCols(ws *xlsxWorksheet, col, offset int) error {
	if ws.Cols == nil {
		return nil
	}
	for i := 0; i < len(ws.Cols.Col); i++ {
		if offset > 0 {
			if ws.Cols.Col[i].Max+1 == col {
				ws.Cols.Col[i].Max += offset
				continue
			}
			if ws.Cols.Col[i].Min >= col {
				ws.Cols.Col[i].Min += offset
				ws.Cols.Col[i].Max += offset
				continue
			}
			if ws.Cols.Col[i].Min < col && ws.Cols.Col[i].Max >= col {
				ws.Cols.Col[i].Max += offset
			}
		}
		if offset < 0 {
			if ws.Cols.Col[i].Min == col && ws.Cols.Col[i].Max == col {
				if len(ws.Cols.Col) > 1 {
					ws.Cols.Col = append(ws.Cols.Col[:i], ws.Cols.Col[i+1:]...)
				} else {
					ws.Cols.Col = nil
				}
				i--
				continue
			}
			if ws.Cols.Col[i].Min > col {
				ws.Cols.Col[i].Min += offset
				ws.Cols.Col[i].Max += offset
				continue
			}
			if ws.Cols.Col[i].Min <= col && ws.Cols.Col[i].Max >= col {
				ws.Cols.Col[i].Max += offset
			}
		}
	}
	return nil
}

// adjustColDimensions provides a function to update column dimensions when
// inserting or deleting rows or columns.
func (f *File) adjustColDimensions(sheet string, ws *xlsxWorksheet, col, offset int) error {
	for rowIdx := range ws.SheetData.Row {
		for _, v := range ws.SheetData.Row[rowIdx].C {
			if cellCol, _, _ := CellNameToCoordinates(v.R); col <= cellCol {
				if newCol := cellCol + offset; newCol > 0 && newCol > MaxColumns {
					return ErrColumnNumber
				}
			}
		}
	}
	for rowIdx := range ws.SheetData.Row {
		for colIdx, v := range ws.SheetData.Row[rowIdx].C {
			if cellCol, cellRow, _ := CellNameToCoordinates(v.R); col <= cellCol {
				if newCol := cellCol + offset; newCol > 0 {
					ws.SheetData.Row[rowIdx].C[colIdx].R, _ = CoordinatesToCellName(newCol, cellRow)
				}
			}
			if err := f.adjustFormula(sheet, ws.SheetData.Row[rowIdx].C[colIdx].F, columns, col, offset, false); err != nil {
				return err
			}
		}
	}
	return f.adjustCols(ws, col, offset)
}

// adjustRowDimensions provides a function to update row dimensions when
// inserting or deleting rows or columns.
func (f *File) adjustRowDimensions(sheet string, ws *xlsxWorksheet, row, offset int) error {
	totalRows := len(ws.SheetData.Row)
	if totalRows == 0 {
		return nil
	}
	lastRow := &ws.SheetData.Row[totalRows-1]
	if newRow := lastRow.R + offset; lastRow.R >= row && newRow > 0 && newRow > TotalRows {
		return ErrMaxRows
	}
	for i := 0; i < len(ws.SheetData.Row); i++ {
		r := &ws.SheetData.Row[i]
		if newRow := r.R + offset; r.R >= row && newRow > 0 {
			if err := f.adjustSingleRowDimensions(sheet, r, row, offset, false); err != nil {
				return err
			}
		}
	}
	return nil
}

// adjustSingleRowDimensions provides a function to adjust single row dimensions.
func (f *File) adjustSingleRowDimensions(sheet string, r *xlsxRow, num, offset int, si bool) error {
	r.R += offset
	for i, col := range r.C {
		colName, _, _ := SplitCellName(col.R)
		r.C[i].R, _ = JoinCellName(colName, r.R)
		if err := f.adjustFormula(sheet, col.F, rows, num, offset, si); err != nil {
			return err
		}
	}
	return nil
}

// adjustFormula provides a function to adjust formula reference and shared
// formula reference.
func (f *File) adjustFormula(sheet string, formula *xlsxF, dir adjustDirection, num, offset int, si bool) error {
	if formula == nil {
		return nil
	}
	adjustRef := func(ref string) (string, error) {
		coordinates, err := rangeRefToCoordinates(ref)
		if err != nil {
			return ref, err
		}
		if dir == columns {
			coordinates[0] += offset
			coordinates[2] += offset
		} else {
			coordinates[1] += offset
			coordinates[3] += offset
		}
		return f.coordinatesToRangeRef(coordinates)
	}
	var err error
	if formula.Ref != "" {
		if formula.Ref, err = adjustRef(formula.Ref); err != nil {
			return err
		}
		if si && formula.Si != nil {
			formula.Si = intPtr(*formula.Si + 1)
		}
	}
	if formula.T == STCellFormulaTypeArray {
		formula.Content, err = adjustRef(strings.TrimPrefix(formula.Content, "="))
		return err
	}
	if formula.Content != "" && !strings.ContainsAny(formula.Content, "[:]") {
		content, err := f.adjustFormulaRef(sheet, formula.Content, dir, num, offset)
		if err != nil {
			return err
		}
		formula.Content = content
	}
	return nil
}

// adjustFormulaRef returns adjusted formula text by giving adjusting direction
// and the base number of column or row, and offset.
func (f *File) adjustFormulaRef(sheet string, text string, dir adjustDirection, num, offset int) (string, error) {
	var (
		formulaText  string
		definedNames []string
		ps           = efp.ExcelParser()
	)
	for _, definedName := range f.GetDefinedName() {
		if definedName.Scope == "Workbook" || definedName.Scope == sheet {
			definedNames = append(definedNames, definedName.Name)
		}
	}
	for _, token := range ps.Parse(text) {
		if token.TType == efp.TokenTypeOperand && token.TSubType == efp.TokenSubTypeRange {
			if inStrSlice(definedNames, token.TValue, true) != -1 {
				formulaText += token.TValue
				continue
			}
			c, r, err := CellNameToCoordinates(token.TValue)
			if err != nil {
				return formulaText, err
			}
			if dir == columns && c >= num {
				c += offset
			}
			if dir == rows {
				r += offset
			}
			cell, err := CoordinatesToCellName(c, r, strings.Contains(token.TValue, "$"))
			if err != nil {
				return formulaText, err
			}
			formulaText += cell
			continue
		}
		formulaText += token.TValue
	}
	return formulaText, nil
}

// adjustHyperlinks provides a function to update hyperlinks when inserting or
// deleting rows or columns.
func (f *File) adjustHyperlinks(ws *xlsxWorksheet, sheet string, dir adjustDirection, num, offset int) {
	// short path
	if ws.Hyperlinks == nil || len(ws.Hyperlinks.Hyperlink) == 0 {
		return
	}

	// order is important
	if offset < 0 {
		for i := len(ws.Hyperlinks.Hyperlink) - 1; i >= 0; i-- {
			linkData := ws.Hyperlinks.Hyperlink[i]
			colNum, rowNum, _ := CellNameToCoordinates(linkData.Ref)

			if (dir == rows && num == rowNum) || (dir == columns && num == colNum) {
				f.deleteSheetRelationships(sheet, linkData.RID)
				if len(ws.Hyperlinks.Hyperlink) > 1 {
					ws.Hyperlinks.Hyperlink = append(ws.Hyperlinks.Hyperlink[:i],
						ws.Hyperlinks.Hyperlink[i+1:]...)
				} else {
					ws.Hyperlinks = nil
				}
			}
		}
	}
	if ws.Hyperlinks == nil {
		return
	}
	for i := range ws.Hyperlinks.Hyperlink {
		link := &ws.Hyperlinks.Hyperlink[i] // get reference
		colNum, rowNum, _ := CellNameToCoordinates(link.Ref)
		if dir == rows {
			if rowNum >= num {
				link.Ref, _ = CoordinatesToCellName(colNum, rowNum+offset)
			}
		} else {
			if colNum >= num {
				link.Ref, _ = CoordinatesToCellName(colNum+offset, rowNum)
			}
		}
	}
}

// adjustTable provides a function to update the table when inserting or
// deleting rows or columns.
func (f *File) adjustTable(ws *xlsxWorksheet, sheet string, dir adjustDirection, num, offset int) {
	if ws.TableParts == nil || len(ws.TableParts.TableParts) == 0 {
		return
	}
	for idx := 0; idx < len(ws.TableParts.TableParts); idx++ {
		tbl := ws.TableParts.TableParts[idx]
		target := f.getSheetRelationshipsTargetByID(sheet, tbl.RID)
		tableXML := strings.ReplaceAll(target, "..", "xl")
		content, ok := f.Pkg.Load(tableXML)
		if !ok {
			continue
		}
		t := xlsxTable{}
		if err := f.xmlNewDecoder(bytes.NewReader(namespaceStrictToTransitional(content.([]byte)))).
			Decode(&t); err != nil && err != io.EOF {
			return
		}
		coordinates, err := rangeRefToCoordinates(t.Ref)
		if err != nil {
			return
		}
		// Remove the table when deleting the header row of the table
		if dir == rows && num == coordinates[0] && offset == -1 {
			ws.TableParts.TableParts = append(ws.TableParts.TableParts[:idx], ws.TableParts.TableParts[idx+1:]...)
			ws.TableParts.Count = len(ws.TableParts.TableParts)
			idx--
			continue
		}
		coordinates = f.adjustAutoFilterHelper(dir, coordinates, num, offset)
		x1, y1, x2, y2 := coordinates[0], coordinates[1], coordinates[2], coordinates[3]
		if y2-y1 < 1 || x2-x1 < 0 {
			ws.TableParts.TableParts = append(ws.TableParts.TableParts[:idx], ws.TableParts.TableParts[idx+1:]...)
			ws.TableParts.Count = len(ws.TableParts.TableParts)
			idx--
			continue
		}
		t.Ref, _ = f.coordinatesToRangeRef([]int{x1, y1, x2, y2})
		if t.AutoFilter != nil {
			t.AutoFilter.Ref = t.Ref
		}
		_, _ = f.setTableHeader(sheet, true, x1, y1, x2)
		table, _ := xml.Marshal(t)
		f.saveFileList(tableXML, table)
	}
}

// adjustAutoFilter provides a function to update the auto filter when
// inserting or deleting rows or columns.
func (f *File) adjustAutoFilter(ws *xlsxWorksheet, dir adjustDirection, num, offset int) error {
	if ws.AutoFilter == nil {
		return nil
	}

	coordinates, err := rangeRefToCoordinates(ws.AutoFilter.Ref)
	if err != nil {
		return err
	}
	x1, y1, x2, y2 := coordinates[0], coordinates[1], coordinates[2], coordinates[3]

	if (dir == rows && y1 == num && offset < 0) || (dir == columns && x1 == num && x2 == num) {
		ws.AutoFilter = nil
		for rowIdx := range ws.SheetData.Row {
			rowData := &ws.SheetData.Row[rowIdx]
			if rowData.R > y1 && rowData.R <= y2 {
				rowData.Hidden = false
			}
		}
		return err
	}

	coordinates = f.adjustAutoFilterHelper(dir, coordinates, num, offset)
	x1, y1, x2, y2 = coordinates[0], coordinates[1], coordinates[2], coordinates[3]

	ws.AutoFilter.Ref, err = f.coordinatesToRangeRef([]int{x1, y1, x2, y2})
	return err
}

// adjustAutoFilterHelper provides a function for adjusting auto filter to
// compare and calculate cell reference by the giving adjusting direction,
// operation reference and offset.
func (f *File) adjustAutoFilterHelper(dir adjustDirection, coordinates []int, num, offset int) []int {
	if dir == rows {
		if coordinates[1] >= num {
			coordinates[1] += offset
		}
		if coordinates[3] >= num {
			coordinates[3] += offset
		}
		return coordinates
	}
	if coordinates[0] >= num {
		coordinates[0] += offset
	}
	if coordinates[2] >= num {
		coordinates[2] += offset
	}
	return coordinates
}

// adjustMergeCells provides a function to update merged cells when inserting
// or deleting rows or columns.
func (f *File) adjustMergeCells(ws *xlsxWorksheet, dir adjustDirection, num, offset int) error {
	if ws.MergeCells == nil {
		return nil
	}

	for i := 0; i < len(ws.MergeCells.Cells); i++ {
		mergedCells := ws.MergeCells.Cells[i]
		mergedCellsRef := mergedCells.Ref
		if !strings.Contains(mergedCellsRef, ":") {
			mergedCellsRef += ":" + mergedCellsRef
		}
		coordinates, err := rangeRefToCoordinates(mergedCellsRef)
		if err != nil {
			return err
		}
		x1, y1, x2, y2 := coordinates[0], coordinates[1], coordinates[2], coordinates[3]
		if dir == rows {
			if y1 == num && y2 == num && offset < 0 {
				f.deleteMergeCell(ws, i)
				i--
				continue
			}

			y1, y2 = f.adjustMergeCellsHelper(y1, y2, num, offset)
		} else {
			if x1 == num && x2 == num && offset < 0 {
				f.deleteMergeCell(ws, i)
				i--
				continue
			}

			x1, x2 = f.adjustMergeCellsHelper(x1, x2, num, offset)
		}
		if x1 == x2 && y1 == y2 {
			f.deleteMergeCell(ws, i)
			i--
			continue
		}
		mergedCells.rect = []int{x1, y1, x2, y2}
		if mergedCells.Ref, err = f.coordinatesToRangeRef([]int{x1, y1, x2, y2}); err != nil {
			return err
		}
	}
	return nil
}

// adjustMergeCellsHelper provides a function for adjusting merge cells to
// compare and calculate cell reference by the given pivot, operation reference and
// offset.
func (f *File) adjustMergeCellsHelper(p1, p2, num, offset int) (int, int) {
	if p2 < p1 {
		p1, p2 = p2, p1
	}

	if offset >= 0 {
		if num <= p1 {
			p1 += offset
			p2 += offset
		} else if num <= p2 {
			p2 += offset
		}
		return p1, p2
	}
	if num < p1 || (num == p1 && num == p2) {
		p1 += offset
		p2 += offset
	} else if num <= p2 {
		p2 += offset
	}
	return p1, p2
}

// deleteMergeCell provides a function to delete merged cell by given index.
func (f *File) deleteMergeCell(ws *xlsxWorksheet, idx int) {
	if idx < 0 {
		return
	}
	if len(ws.MergeCells.Cells) > idx {
		ws.MergeCells.Cells = append(ws.MergeCells.Cells[:idx], ws.MergeCells.Cells[idx+1:]...)
		ws.MergeCells.Count = len(ws.MergeCells.Cells)
	}
}

// adjustCalcChainRef update the cell reference in calculation chain when
// inserting or deleting rows or columns.
func (f *File) adjustCalcChainRef(i, c, r, offset int, dir adjustDirection) {
	if dir == rows {
		if rn := r + offset; rn > 0 {
			f.CalcChain.C[i].R, _ = CoordinatesToCellName(c, rn)
		}
		return
	}
	if nc := c + offset; nc > 0 {
		f.CalcChain.C[i].R, _ = CoordinatesToCellName(nc, r)
	}
}

// adjustCalcChain provides a function to update the calculation chain when
// inserting or deleting rows or columns.
func (f *File) adjustCalcChain(dir adjustDirection, num, offset, sheetID int) error {
	if f.CalcChain == nil {
		return nil
	}
	// If sheet ID is omitted, it is assumed to be the same as the i value of
	// the previous cell.
	var prevSheetID int
	for index, c := range f.CalcChain.C {
		if c.I == 0 {
			c.I = prevSheetID
		}
		prevSheetID = c.I
		if c.I != sheetID {
			continue
		}
		colNum, rowNum, err := CellNameToCoordinates(c.R)
		if err != nil {
			return err
		}
		if dir == rows && num <= rowNum {
			if num == rowNum && offset == -1 {
				_ = f.deleteCalcChain(c.I, c.R)
				continue
			}
			f.adjustCalcChainRef(index, colNum, rowNum, offset, dir)
		}
		if dir == columns && num <= colNum {
			if num == colNum && offset == -1 {
				_ = f.deleteCalcChain(c.I, c.R)
				continue
			}
			f.adjustCalcChainRef(index, colNum, rowNum, offset, dir)
		}
	}
	return nil
}
