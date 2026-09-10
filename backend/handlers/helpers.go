package handlers

import (
	"net/http"

	"github.com/xuri/excelize/v2"
)

func writeXlsxResponse(w http.ResponseWriter, f *excelize.File, filename string) {
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	if err := f.Write(w); err != nil {
		http.Error(w, "gagal membuat file excel", http.StatusInternalServerError)
	}
}
