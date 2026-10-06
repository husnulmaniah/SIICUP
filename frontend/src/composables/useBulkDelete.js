import { ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'

// useBulkDelete: pola seragam "centang beberapa baris -> klik satu tombol
// Hapus Terpilih" yang dipakai di banyak halaman (Master Data lewat
// CrudManager.vue, Pengajuan Cuti, Rekap Absen, Surat Rekomendasi, Template
// Surat, Shift Kerja, Penerima TPP, dst). Backend TIDAK punya endpoint bulk
// delete di mana pun -- jadi di sini cukup memanggil endpoint hapus-satuan
// yang SUDAH ADA berkali-kali secara paralel (Promise.allSettled, supaya
// satu baris gagal dihapus tidak membatalkan/menghentikan baris lain), lalu
// menampilkan ringkasan hasil dan memanggil ulang pemuatan data satu kali di
// akhir -- bukan menambah endpoint baru di backend.
//
// Dipakai di <script setup> (bukan di luar komponen) karena memanggil
// useToast()/useConfirm() yang butuh konteks Vue aktif, sama seperti kalau
// dipanggil langsung di komponen.
export function useBulkDelete() {
  const toast = useToast()
  const confirm = useConfirm()
  // array baris (objek) yang sedang dicentang -- pasang langsung ke
  // v-model:selection pada <DataTable>, atau kelola manual (push/splice)
  // untuk tampilan card grid yang tidak memakai <DataTable>.
  const selected = ref([])
  const bulkDeleting = ref(false)

  // confirmBulkDelete menampilkan dialog konfirmasi, lalu kalau disetujui,
  // menghapus seluruh baris di `selected` satu per satu (paralel) lewat
  // `deleteOne(item)` yang disediakan pemanggil (biasanya cuma pembungkus
  // http.delete ke endpoint hapus-satuan yang sudah ada untuk halaman itu).
  //
  // Opsi:
  // - label: kata benda tunggal untuk ditampilkan di pesan (mis. "jabatan",
  //   "pengajuan cuti", "data absen").
  // - deleteOne(item): async function, melempar error kalau gagal (dipakai
  //   untuk menghitung baris yang gagal, BUKAN menghentikan baris lainnya).
  // - onDone(): dipanggil sekali di akhir (berhasil penuh/sebagian) untuk
  //   memuat ulang daftar & mengosongkan state lain di komponen pemanggil.
  // - extraMessage: kalimat tambahan opsional di pesan konfirmasi (mis.
  //   peringatan konsekuensi khusus halaman itu).
  function confirmBulkDelete({ label = 'data', deleteOne, onDone, extraMessage = '' }) {
    if (!selected.value.length) {
      toast.add({
        severity: 'warn',
        summary: 'Belum ada yang dipilih',
        detail: `Centang dulu ${label} yang mau dihapus`,
        life: 4000,
      })
      return
    }
    const jumlah = selected.value.length
    confirm.require({
      message: `Hapus ${jumlah} ${label} terpilih? Tindakan ini tidak bisa dibatalkan.${extraMessage ? ' ' + extraMessage : ''}`,
      header: 'Konfirmasi Hapus Terpilih',
      icon: 'pi pi-exclamation-triangle',
      acceptLabel: 'Ya, Hapus Semua',
      rejectLabel: 'Batal',
      acceptClass: 'p-button-danger',
      accept: async () => {
        bulkDeleting.value = true
        const items = [...selected.value]
        const results = await Promise.allSettled(items.map((item) => deleteOne(item)))
        const gagal = results.filter((r) => r.status === 'rejected')
        const berhasil = results.length - gagal.length
        if (gagal.length === 0) {
          toast.add({ severity: 'success', summary: 'Berhasil', detail: `${berhasil} ${label} berhasil dihapus`, life: 5000 })
        } else if (berhasil === 0) {
          toast.add({ severity: 'error', summary: 'Gagal', detail: `Semua ${items.length} ${label} gagal dihapus`, life: 6000 })
        } else {
          toast.add({
            severity: 'warn',
            summary: 'Sebagian berhasil',
            detail: `${berhasil} berhasil dihapus, ${gagal.length} gagal dihapus`,
            life: 6000,
          })
        }
        selected.value = []
        bulkDeleting.value = false
        if (onDone) await onDone()
      },
    })
  }

  return { selected, bulkDeleting, confirmBulkDelete }
}
