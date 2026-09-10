package handlers

import (
	"net/http"

	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

func writeXlsxResponse(w http.ResponseWriter, f *excelize.File, filename string) {
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	if err := f.Write(w); err != nil {
		http.Error(w, "gagal membuat file excel", http.StatusInternalServerError)
	}
}

// isReplaceMode reads the "mode" form field sent alongside an excel import
// upload. mode=replace means: wipe all existing rows of this table first,
// then import. Anything else (including empty/missing) means append-only,
// which is the default/original behaviour.
func isReplaceMode(r *http.Request) bool {
	return r.FormValue("mode") == "replace"
}

// deleteAllRows wipes every row of model T (e.g. &models.Pegawai{}) before a
// "replace" import. Returns a human-readable error (e.g. blocked by a
// foreign key from another table) so the import can be aborted cleanly
// without touching any data.
func deleteAllRows(db *gorm.DB, model interface{}) error {
	return db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(model).Error
}
