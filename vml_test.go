// Copyright 2016 - 2024 The excelize Authors. All rights reserved. Use of
// this source code is governed by a BSD-style license that can be found in
// the LICENSE file.
//
// Package excelize providing a set of functions that allow you to write to and
// read from XLAM / XLSM / XLSX / XLTM / XLTX files. Supports reading and
// writing spreadsheet documents generated by Microsoft Excel™ 2007 and later.
// Supports complex components by high compatibility, and provided streaming
// API for generating or reading data from a worksheet with huge amounts of
// data. This library needs Go version 1.18 or later.

package excelize

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddComment(t *testing.T) {
	f, err := prepareTestBook1()
	if !assert.NoError(t, err) {
		t.FailNow()
	}

	s := strings.Repeat("c", TotalCellChars+1)
	assert.NoError(t, f.AddComment("Sheet1", Comment{Cell: "A30", Author: s, Text: s, Paragraph: []RichTextRun{{Text: s}, {Text: s}}}))
	assert.NoError(t, f.AddComment("Sheet2", Comment{Cell: "B7", Author: "Excelize", Text: s[:TotalCellChars-1], Paragraph: []RichTextRun{{Text: "Excelize: ", Font: &Font{Bold: true}}, {Text: "This is a comment."}}}))

	// Test add comment on not exists worksheet
	assert.EqualError(t, f.AddComment("SheetN", Comment{Cell: "B7", Author: "Excelize", Paragraph: []RichTextRun{{Text: "Excelize: ", Font: &Font{Bold: true}}, {Text: "This is a comment."}}}), "sheet SheetN does not exist")
	// Test add comment on with illegal cell reference
	assert.Equal(t, newCellNameToCoordinatesError("A", newInvalidCellNameError("A")), f.AddComment("Sheet1", Comment{Cell: "A", Author: "Excelize", Paragraph: []RichTextRun{{Text: "Excelize: ", Font: &Font{Bold: true}}, {Text: "This is a comment."}}}))
	comments, err := f.GetComments("Sheet1")
	assert.NoError(t, err)
	assert.Len(t, comments, 2)
	comments, err = f.GetComments("Sheet2")
	assert.NoError(t, err)
	assert.Len(t, comments, 1)
	assert.NoError(t, f.SaveAs(filepath.Join("test", "TestAddComments.xlsx")))

	f.Comments["xl/comments2.xml"] = nil
	f.Pkg.Store("xl/comments2.xml", []byte(xml.Header+`<comments xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><authors><author>Excelize: </author></authors><commentList><comment ref="B7" authorId="0"><text><t>Excelize: </t></text></comment></commentList></comments>`))
	comments, err = f.GetComments("Sheet1")
	assert.NoError(t, err)
	assert.Len(t, comments, 2)
	comments, err = f.GetComments("Sheet2")
	assert.NoError(t, err)
	assert.Len(t, comments, 1)
	comments, err = NewFile().GetComments("Sheet1")
	assert.NoError(t, err)
	assert.Len(t, comments, 0)

	// Test add comments with invalid sheet name
	assert.Equal(t, ErrSheetNameInvalid, f.AddComment("Sheet:1", Comment{Cell: "A1", Author: "Excelize", Text: "This is a comment."}))

	// Test add comments with unsupported charset
	f.Comments["xl/comments2.xml"] = nil
	f.Pkg.Store("xl/comments2.xml", MacintoshCyrillicCharset)
	_, err = f.GetComments("Sheet2")
	assert.EqualError(t, err, "XML syntax error on line 1: invalid UTF-8")

	// Test add comments with unsupported charset
	f.Comments["xl/comments2.xml"] = nil
	f.Pkg.Store("xl/comments2.xml", MacintoshCyrillicCharset)
	assert.EqualError(t, f.AddComment("Sheet2", Comment{Cell: "A30", Text: "Comment"}), "XML syntax error on line 1: invalid UTF-8")

	// Test add comments with unsupported charset style sheet
	f.Styles = nil
	f.Pkg.Store(defaultXMLPathStyles, MacintoshCyrillicCharset)
	assert.EqualError(t, f.AddComment("Sheet2", Comment{Cell: "A30", Text: "Comment"}), "XML syntax error on line 1: invalid UTF-8")

	// Test get comments on not exists worksheet
	comments, err = f.GetComments("SheetN")
	assert.Len(t, comments, 0)
	assert.EqualError(t, err, "sheet SheetN does not exist")
}

func TestDeleteComment(t *testing.T) {
	f, err := prepareTestBook1()
	if !assert.NoError(t, err) {
		t.FailNow()
	}

	assert.NoError(t, f.AddComment("Sheet2", Comment{Cell: "A40", Text: "Excelize: This is a comment1."}))
	assert.NoError(t, f.AddComment("Sheet2", Comment{Cell: "A41", Paragraph: []RichTextRun{{Text: "Excelize: ", Font: &Font{Bold: true}}, {Text: "This is a comment2."}}}))
	assert.NoError(t, f.AddComment("Sheet2", Comment{Cell: "C41", Paragraph: []RichTextRun{{Text: "Excelize: ", Font: &Font{Bold: true}}, {Text: "This is a comment3."}}}))
	assert.NoError(t, f.AddComment("Sheet2", Comment{Cell: "C41", Paragraph: []RichTextRun{{Text: "Excelize: ", Font: &Font{Bold: true}}, {Text: "This is a comment3-1."}}}))
	assert.NoError(t, f.AddComment("Sheet2", Comment{Cell: "C42", Paragraph: []RichTextRun{{Text: "Excelize: ", Font: &Font{Bold: true}}, {Text: "This is a comment4."}}}))
	assert.NoError(t, f.AddComment("Sheet2", Comment{Cell: "C41", Paragraph: []RichTextRun{{Text: "Excelize: ", Font: &Font{Bold: true}}, {Text: "This is a comment2."}}}))

	assert.NoError(t, f.DeleteComment("Sheet2", "A40"))

	comments, err := f.GetComments("Sheet2")
	assert.NoError(t, err)
	assert.Len(t, comments, 5)

	comments, err = NewFile().GetComments("Sheet1")
	assert.NoError(t, err)
	assert.Len(t, comments, 0)

	// Test delete comment with invalid sheet name
	assert.Equal(t, ErrSheetNameInvalid, f.DeleteComment("Sheet:1", "A1"))
	// Test delete all comments in a worksheet
	assert.NoError(t, f.DeleteComment("Sheet2", "A41"))
	assert.NoError(t, f.DeleteComment("Sheet2", "C41"))
	assert.NoError(t, f.DeleteComment("Sheet2", "C42"))
	comments, err = f.GetComments("Sheet2")
	assert.NoError(t, err)
	assert.EqualValues(t, 0, len(comments))
	// Test delete comment on not exists worksheet
	assert.EqualError(t, f.DeleteComment("SheetN", "A1"), "sheet SheetN does not exist")
	// Test delete comment with worksheet part
	f.Pkg.Delete("xl/worksheets/sheet1.xml")
	assert.NoError(t, f.DeleteComment("Sheet1", "A22"))

	f.Comments["xl/comments2.xml"] = nil
	f.Pkg.Store("xl/comments2.xml", MacintoshCyrillicCharset)
	assert.EqualError(t, f.DeleteComment("Sheet2", "A41"), "XML syntax error on line 1: invalid UTF-8")
}

func TestDecodeVMLDrawingReader(t *testing.T) {
	f := NewFile()
	path := "xl/drawings/vmlDrawing1.xml"
	f.Pkg.Store(path, MacintoshCyrillicCharset)
	_, err := f.decodeVMLDrawingReader(path)
	assert.EqualError(t, err, "XML syntax error on line 1: invalid UTF-8")
}

func TestCommentsReader(t *testing.T) {
	f := NewFile()
	// Test read comments with unsupported charset
	path := "xl/comments1.xml"
	f.Pkg.Store(path, MacintoshCyrillicCharset)
	_, err := f.commentsReader(path)
	assert.EqualError(t, err, "XML syntax error on line 1: invalid UTF-8")
}

func TestCountComments(t *testing.T) {
	f := NewFile()
	f.Comments["xl/comments1.xml"] = nil
	assert.Equal(t, f.countComments(), 1)
}

func TestAddDrawingVML(t *testing.T) {
	// Test addDrawingVML with illegal cell reference
	f := NewFile()
	assert.Equal(t, f.addDrawingVML(0, "", &vmlOptions{FormControl: FormControl{Cell: "*"}}), newCellNameToCoordinatesError("*", newInvalidCellNameError("*")))

	f.Pkg.Store("xl/drawings/vmlDrawing1.vml", MacintoshCyrillicCharset)
	assert.EqualError(t, f.addDrawingVML(0, "xl/drawings/vmlDrawing1.vml", &vmlOptions{sheet: "Sheet1", FormControl: FormControl{Cell: "A1"}}), "XML syntax error on line 1: invalid UTF-8")
}

func TestFormControl(t *testing.T) {
	f := NewFile()
	formControls := []FormControl{
		{
			Cell: "D1", Type: FormControlButton, Macro: "Button1_Click",
		},
		{
			Cell: "A1", Type: FormControlButton, Macro: "Button1_Click",
			Width: 140, Height: 60, Text: "Button 1\n",
			Paragraph: []RichTextRun{
				{
					Font: &Font{
						Bold:      true,
						Italic:    true,
						Underline: "single",
						Family:    "Times New Roman",
						Size:      14,
						Color:     "777777",
					},
					Text: "C1=A1+B1",
				},
			},
			Format: GraphicOptions{PrintObject: boolPtr(true), Positioning: "absolute"},
		},
		{
			Cell: "A5", Type: FormControlCheckBox, Text: "Check Box 1",
			Checked: true, Format: GraphicOptions{
				PrintObject: boolPtr(false), Positioning: "oneCell",
			},
		},
		{
			Cell: "A6", Type: FormControlCheckBox, Text: "Check Box 2",
			Format: GraphicOptions{Positioning: "twoCell"},
		},
		{
			Cell: "A7", Type: FormControlOptionButton, Text: "Option Button 1", Checked: true,
		},
		{
			Cell: "A8", Type: FormControlOptionButton, Text: "Option Button 2",
		},
		{
			Cell: "D3", Type: FormControlGroupBox, Text: "Group Box 1",
			Width: 140, Height: 60,
		},
		{
			Cell: "A9", Type: FormControlLabel, Text: "Label 1", Width: 140,
		},
		{
			Cell: "C5", Type: FormControlSpinButton, Width: 40, Height: 60,
			CurrentVal: 7, MinVal: 5, MaxVal: 10, IncChange: 1, CellLink: "C2",
		},
		{
			Cell: "D7", Type: FormControlScrollBar, Width: 140, Height: 20,
			CurrentVal: 50, MinVal: 10, MaxVal: 100, IncChange: 1, PageChange: 1, Horizontally: true, CellLink: "C3",
		},
		{
			Cell: "G1", Type: FormControlScrollBar, Width: 20, Height: 140,
			CurrentVal: 50, MinVal: 1000, MaxVal: 100, IncChange: 1, PageChange: 1, CellLink: "C4",
		},
	}
	for _, formCtrl := range formControls {
		assert.NoError(t, f.AddFormControl("Sheet1", formCtrl))
	}
	// Test get from controls
	result, err := f.GetFormControls("Sheet1")
	assert.NoError(t, err)
	assert.Len(t, result, 11)
	for i, formCtrl := range formControls {
		assert.Equal(t, formCtrl.Type, result[i].Type)
		assert.Equal(t, formCtrl.Cell, result[i].Cell)
		assert.Equal(t, formCtrl.Macro, result[i].Macro)
		assert.Equal(t, formCtrl.Checked, result[i].Checked)
		assert.Equal(t, formCtrl.CurrentVal, result[i].CurrentVal)
		assert.Equal(t, formCtrl.MinVal, result[i].MinVal)
		assert.Equal(t, formCtrl.MaxVal, result[i].MaxVal)
		assert.Equal(t, formCtrl.IncChange, result[i].IncChange)
		assert.Equal(t, formCtrl.Horizontally, result[i].Horizontally)
		assert.Equal(t, formCtrl.CellLink, result[i].CellLink)
		assert.Equal(t, formCtrl.Text, result[i].Text)
		assert.Equal(t, len(formCtrl.Paragraph), len(result[i].Paragraph))
	}
	assert.NoError(t, f.SetSheetProps("Sheet1", &SheetPropsOptions{CodeName: stringPtr("Sheet1")}))
	file, err := os.ReadFile(filepath.Join("test", "vbaProject.bin"))
	assert.NoError(t, err)
	assert.NoError(t, f.AddVBAProject(file))
	assert.NoError(t, f.SaveAs(filepath.Join("test", "TestAddFormControl.xlsm")))
	assert.NoError(t, f.Close())
	f, err = OpenFile(filepath.Join("test", "TestAddFormControl.xlsm"))
	assert.NoError(t, err)
	// Test get from controls before add form controls
	result, err = f.GetFormControls("Sheet1")
	assert.NoError(t, err)
	assert.Len(t, result, 11)
	// Test add from control to a worksheet which already contains form controls
	assert.NoError(t, f.AddFormControl("Sheet1", FormControl{
		Cell: "D4", Type: FormControlButton, Macro: "Button1_Click",
		Paragraph: []RichTextRun{{Font: &Font{Underline: "double"}, Text: "Button 2"}},
	}))
	// Test get from controls after add form controls
	result, err = f.GetFormControls("Sheet1")
	assert.NoError(t, err)
	assert.Len(t, result, 12)
	// Test add unsupported form control
	assert.Equal(t, f.AddFormControl("Sheet1", FormControl{
		Cell: "A1", Type: 0x37, Macro: "Button1_Click",
	}), ErrParameterInvalid)
	// Test add form control on not exists worksheet
	assert.Equal(t, ErrSheetNotExist{"SheetN"}, f.AddFormControl("SheetN", FormControl{
		Cell: "A1", Type: FormControlButton, Macro: "Button1_Click",
	}))
	// Test add form control with invalid positioning types
	assert.Equal(t, f.AddFormControl("Sheet1", FormControl{
		Cell: "A1", Type: FormControlButton,
		Format: GraphicOptions{Positioning: "x"},
	}), ErrParameterInvalid)
	// Test add spin form control with illegal cell link reference
	assert.Equal(t, f.AddFormControl("Sheet1", FormControl{
		Cell: "C5", Type: FormControlSpinButton, CellLink: "*",
	}), newCellNameToCoordinatesError("*", newInvalidCellNameError("*")))
	// Test add spin form control with invalid scroll value
	assert.Equal(t, f.AddFormControl("Sheet1", FormControl{
		Cell: "C5", Type: FormControlSpinButton, CurrentVal: MaxFormControlValue + 1,
	}), ErrFormControlValue)
	assert.NoError(t, f.Close())
	// Test delete form control
	f, err = OpenFile(filepath.Join("test", "TestAddFormControl.xlsm"))
	assert.NoError(t, err)
	assert.NoError(t, f.DeleteFormControl("Sheet1", "D1"))
	assert.NoError(t, f.DeleteFormControl("Sheet1", "A1"))
	// Test get from controls after delete form controls
	result, err = f.GetFormControls("Sheet1")
	assert.NoError(t, err)
	assert.Len(t, result, 9)
	// Test delete form control on not exists worksheet
	assert.Equal(t, ErrSheetNotExist{"SheetN"}, f.DeleteFormControl("SheetN", "A1"))
	// Test delete form control with illegal cell link reference
	assert.Equal(t, f.DeleteFormControl("Sheet1", "A"), newCellNameToCoordinatesError("A", newInvalidCellNameError("A")))
	assert.NoError(t, f.SaveAs(filepath.Join("test", "TestDeleteFormControl.xlsm")))
	assert.NoError(t, f.Close())
	// Test delete form control with expected element
	f, err = OpenFile(filepath.Join("test", "TestAddFormControl.xlsm"))
	assert.NoError(t, err)
	f.Pkg.Store("xl/drawings/vmlDrawing1.vml", MacintoshCyrillicCharset)
	assert.Error(t, f.DeleteFormControl("Sheet1", "A1"), "XML syntax error on line 1: invalid UTF-8")
	// Test delete form controls with invalid shape anchor
	f.DecodeVMLDrawing["xl/drawings/vmlDrawing1.vml"] = &decodeVmlDrawing{
		Shape: []decodeShape{{Type: "#_x0000_t201", Val: "<x:ClientData ObjectType=\"Scroll\"><x:Anchor>0</x:Anchor></x:ClientData>"}},
	}
	assert.Equal(t, ErrParameterInvalid, f.DeleteFormControl("Sheet1", "A1"))
	assert.NoError(t, f.Close())
	// Test delete form control on a worksheet without form control
	f = NewFile()
	assert.NoError(t, f.DeleteFormControl("Sheet1", "A1"))
	// Test get form controls on a worksheet without form control
	_, err = f.GetFormControls("Sheet1")
	assert.NoError(t, err)
	// Test get form controls on not exists worksheet
	_, err = f.GetFormControls("SheetN")
	assert.Equal(t, ErrSheetNotExist{"SheetN"}, err)
	// Test get form controls with unsupported charset VML drawing
	f, err = OpenFile(filepath.Join("test", "TestAddFormControl.xlsm"))
	assert.NoError(t, err)
	f.Pkg.Store("xl/drawings/vmlDrawing1.vml", MacintoshCyrillicCharset)
	_, err = f.GetFormControls("Sheet1")
	assert.EqualError(t, err, "XML syntax error on line 1: invalid UTF-8")
	// Test get form controls with unsupported shape type
	f.DecodeVMLDrawing["xl/drawings/vmlDrawing1.vml"] = &decodeVmlDrawing{
		Shape: []decodeShape{{Type: "_x0000_t202"}},
	}
	formControls, err = f.GetFormControls("Sheet1")
	assert.NoError(t, err)
	assert.Len(t, formControls, 0)
	// Test get form controls with bold font format
	f.DecodeVMLDrawing["xl/drawings/vmlDrawing1.vml"] = &decodeVmlDrawing{
		Shape: []decodeShape{{Type: "#_x0000_t201", Val: "<v:textbox><div><font><b>Text</b></font></div></v:textbox><x:ClientData ObjectType=\"Scroll\"><x:Anchor>0,0,0,0,0,0,0,0</x:Anchor></x:ClientData>"}},
	}
	formControls, err = f.GetFormControls("Sheet1")
	assert.NoError(t, err)
	assert.True(t, formControls[0].Paragraph[0].Font.Bold)
	// Test get form controls with italic font format
	f.DecodeVMLDrawing["xl/drawings/vmlDrawing1.vml"] = &decodeVmlDrawing{
		Shape: []decodeShape{{Type: "#_x0000_t201", Val: "<v:textbox><div><font><i>Text</i></font></div></v:textbox><x:ClientData ObjectType=\"Scroll\"><x:Anchor>0,0,0,0,0,0,0,0</x:Anchor></x:ClientData>"}},
	}
	formControls, err = f.GetFormControls("Sheet1")
	assert.NoError(t, err)
	assert.True(t, formControls[0].Paragraph[0].Font.Italic)
	// Test get form controls with font format
	f.DecodeVMLDrawing["xl/drawings/vmlDrawing1.vml"] = &decodeVmlDrawing{
		Shape: []decodeShape{{Type: "#_x0000_t201", Val: "<v:textbox><div><font face=\"Calibri\" size=\"280\" color=\"#777777\">Text</font></div></v:textbox><x:ClientData ObjectType=\"Scroll\"><x:Anchor>0,0,0,0,0,0,0,0</x:Anchor></x:ClientData>"}},
	}
	formControls, err = f.GetFormControls("Sheet1")
	assert.NoError(t, err)
	assert.Equal(t, "Calibri", formControls[0].Paragraph[0].Font.Family)
	assert.Equal(t, 14.0, formControls[0].Paragraph[0].Font.Size)
	assert.Equal(t, "#777777", formControls[0].Paragraph[0].Font.Color)
	// Test get form controls with italic font format
	f.DecodeVMLDrawing["xl/drawings/vmlDrawing1.vml"] = &decodeVmlDrawing{
		Shape: []decodeShape{{Type: "#_x0000_t201", Val: "<v:textbox><div><font><i>Text</i></font></div></v:textbox><x:ClientData ObjectType=\"Scroll\"><x:Anchor>0,0,0,0,0,0,0,0</x:Anchor></x:ClientData>"}},
	}
	formControls, err = f.GetFormControls("Sheet1")
	assert.NoError(t, err)
	assert.True(t, formControls[0].Paragraph[0].Font.Italic)
	// Test get form controls with invalid column number
	f.DecodeVMLDrawing["xl/drawings/vmlDrawing1.vml"] = &decodeVmlDrawing{
		Shape: []decodeShape{{Type: "#_x0000_t201", Val: fmt.Sprintf("<x:ClientData ObjectType=\"Scroll\"><x:Anchor>%d,0,0,0,0,0,0,0</x:Anchor></x:ClientData>", MaxColumns)}},
	}
	formControls, err = f.GetFormControls("Sheet1")
	assert.Equal(t, err, ErrColumnNumber)
	assert.Len(t, formControls, 0)
	// Test get form controls with comment (Note) shape type
	f.DecodeVMLDrawing["xl/drawings/vmlDrawing1.vml"] = &decodeVmlDrawing{
		Shape: []decodeShape{{Type: "#_x0000_t201", Val: "<x:ClientData ObjectType=\"Note\"></x:ClientData>"}},
	}
	formControls, err = f.GetFormControls("Sheet1")
	assert.NoError(t, err)
	assert.Len(t, formControls, 0)
	// Test get form controls with unsupported shape type
	f.VMLDrawing["xl/drawings/vmlDrawing1.vml"] = &vmlDrawing{
		Shape: []xlsxShape{{Type: "_x0000_t202"}},
	}
	formControls, err = f.GetFormControls("Sheet1")
	assert.NoError(t, err)
	assert.Len(t, formControls, 0)
	// Test get form controls with invalid column number
	f.VMLDrawing["xl/drawings/vmlDrawing1.vml"] = &vmlDrawing{
		Shape: []xlsxShape{{Type: "#_x0000_t201", Val: fmt.Sprintf("<x:ClientData ObjectType=\"Scroll\"><x:Anchor>%d,0,0,0,0,0,0,0</x:Anchor></x:ClientData>", MaxColumns)}},
	}
	formControls, err = f.GetFormControls("Sheet1")
	assert.Equal(t, err, ErrColumnNumber)
	assert.Len(t, formControls, 0)
	// Test get form controls with invalid shape anchor
	f.VMLDrawing["xl/drawings/vmlDrawing1.vml"] = &vmlDrawing{
		Shape: []xlsxShape{{Type: "#_x0000_t201", Val: "<x:ClientData ObjectType=\"Scroll\"><x:Anchor>x,0,0,0,0,0,0,0</x:Anchor></x:ClientData>"}},
	}
	formControls, err = f.GetFormControls("Sheet1")
	assert.Equal(t, ErrColumnNumber, err)
	assert.Len(t, formControls, 0)
	// Test get form controls with comment (Note) shape type
	f.VMLDrawing["xl/drawings/vmlDrawing1.vml"] = &vmlDrawing{
		Shape: []xlsxShape{{Type: "#_x0000_t201", Val: "<x:ClientData ObjectType=\"Note\"></x:ClientData>"}},
	}
	formControls, err = f.GetFormControls("Sheet1")
	assert.NoError(t, err)
	assert.Len(t, formControls, 0)
	assert.NoError(t, f.Close())
}

func TestExtractFormControl(t *testing.T) {
	// Test extract form control with unsupported charset
	_, err := extractFormControl(string(MacintoshCyrillicCharset))
	assert.EqualError(t, err, "XML syntax error on line 1: invalid UTF-8")
}
