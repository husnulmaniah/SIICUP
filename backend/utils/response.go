package utils

import (
	"encoding/json"
	"net/http"
)

type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Meta    interface{} `json:"meta,omitempty"`
}

func JSON(w http.ResponseWriter, status int, payload APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func Success(w http.ResponseWriter, message string, data interface{}) {
	JSON(w, http.StatusOK, APIResponse{Success: true, Message: message, Data: data})
}

func SuccessMeta(w http.ResponseWriter, message string, data interface{}, meta interface{}) {
	JSON(w, http.StatusOK, APIResponse{Success: true, Message: message, Data: data, Meta: meta})
}

func Created(w http.ResponseWriter, message string, data interface{}) {
	JSON(w, http.StatusCreated, APIResponse{Success: true, Message: message, Data: data})
}

func Error(w http.ResponseWriter, status int, message string) {
	JSON(w, status, APIResponse{Success: false, Message: message})
}

// LimitBody membatasi ukuran body request SEBELUM dibaca (lewat
// http.MaxBytesReader) -- WAJIB dipanggil di setiap handler upload/
// multipart, tepat sebelum ParseMultipartForm/ParseForm, dengan limit yang
// SAMA dengan limit yang dipakai pada ParseMultipartForm(n) di baris
// setelahnya.
//
// Alasannya: ParseMultipartForm(n) SENDIRIAN (tanpa ini) hanya membatasi
// berapa banyak bagian file yang disimpan di MEMORI -- sisanya tetap
// ditulis ke file sementara di disk dan tetap lolos begitu saja, TIDAK
// error. Jadi pesan error seperti "maksimal 15MB" yang sudah ada di banyak
// handler sebenarnya tidak menegakkan apa-apa: upload jauh lebih besar
// tetap diterima penuh (boros memori/disk & waktu), dan karena backend ini
// berjalan di belakang reverse proxy platform (mis. Railway) yang punya
// batas/timeout koneksi sendiri di luar kontrol kode ini, upload
// besar/lambat itu berisiko diputus paksa SEBELUM backend sempat membalas
// -- di browser ini muncul sebagai "Network Error" generik (axios: request
// gagal total, tidak ada response HTTP sama sekali), bukan pesan error JSON
// yang jelas seperti yang dimaksud handler.
//
// Dengan LimitBody dipasang, begitu body melebihi limit, ParseMultipartForm
// akan gagal LEBIH CEPAT dengan error dari http.MaxBytesReader, dan handler
// tetap bisa membalas dengan pesan error JSON yang sudah ada (mis.
// "maksimal 15MB") alih-alih membiarkan koneksi menggantung lama lalu
// terputus paksa oleh infrastruktur di luar kode ini.
func LimitBody(w http.ResponseWriter, r *http.Request, maxBytes int64) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
}
