import { ref } from 'vue'
import * as faceapi from '@vladmandic/face-api'

// useBlinkLiveness.js -- verifikasi kedipan mata (liveness check) sederhana
// dari stream kamera, dipakai oleh menu Absen (AbsensiView.vue) supaya foto
// absen hanya diambil otomatis begitu terdeteksi mata pegawai berkedip
// (bukan sekadar foto diam/statis).
//
// Cara kerja: setiap ~200ms mengambil satu frame dari elemen <video>, lewat
// face-api.js (fork @vladmandic/face-api yang model weight-nya dibundle
// langsung di public/models -- lihat models yang di-load lewat loadFromUri)
// mendeteksi wajah + 68 titik landmark, menghitung Eye Aspect Ratio (EAR)
// tiap mata dari 6 titik kontur mata (formula standar Soukupova & Cech).
// EAR turun tajam saat mata menutup lalu naik lagi saat terbuka -- transisi
// tertutup->terbuka dalam waktu singkat itulah yang dihitung sebagai satu
// kedipan.
//
// Model di-load SEKALI saja untuk seluruh sesi aplikasi (modelsLoadPromise
// di-cache di level modul), supaya membuka menu Absen berkali-kali tidak
// mengunduh ulang ~550KB model tiap kali.
//
// -- Ambang batas ADAPTIF (bukan nilai EAR mutlak tetap) --
// Versi sebelumnya membandingkan EAR terhadap nilai mutlak tetap (mis. "mata
// tertutup kalau EAR < 0.21"). Ini TIDAK cukup andal karena EAR mata terbuka
// normal berbeda-beda antar pegawai (bentuk mata, sudut kamera, pakai
// kacamata) dan antar kondisi cahaya (absen pulang sore/malam hari sering
// lebih redup daripada absen masuk pagi) -- skala EAR ikut bergeser, bukan
// cuma nilainya. Pegawai yang mata/kondisinya menghasilkan EAR terbuka lebih
// rendah dari rata-rata (mis. 0.24) akan SELALU gagal terdeteksi berkedip
// dengan ambang mutlak, walau kedipannya sebenarnya sama jelasnya.
//
// Sekarang dipakai baseline EAR "mata terbuka" yang dihitung & diperbarui
// SENDIRI selama sesi berjalan (exponential moving average, hanya diperbarui
// saat mata dianggap terbuka supaya baseline tidak ikut turun saat berkedip),
// lalu kedipan dideteksi relatif terhadap baseline itu (EAR turun ke bawah
// ~72% baseline = tertutup, naik lagi ke atas ~85% baseline = terbuka).
// Dengan begitu ambang batas otomatis menyesuaikan diri ke wajah & kondisi
// cahaya pegawai yang sedang absen saat itu.
const EAR_OPEN_RATIO = 0.85
const EAR_CLOSED_RATIO = 0.72
// Klem baseline ke rentang EAR mata terbuka yang wajar secara umum, supaya
// kalau frame pertama kebetulan menangkap mata separuh tertutup/silau,
// baseline tidak jadi terlalu ekstrem (yang akan membuat blink mustahil
// terdeteksi, atau sebaliknya terlalu sensitif).
const EAR_BASELINE_MIN = 0.16
const EAR_BASELINE_MAX = 0.42
const BASELINE_EMA_ALPHA = 0.12
function clampBaseline(v) {
  return Math.min(EAR_BASELINE_MAX, Math.max(EAR_BASELINE_MIN, v))
}

// Kedipan wajar berlangsung < 400ms, tapi deteksi kita hanya sampling tiap
// ~200ms jadi kita beri toleransi jendela waktu tertutup->terbuka sampai 1.5s
// (mencakup jeda antar-sampling + kedipan yang agak lambat).
const MAX_CLOSED_MS = 1500

function dist(a, b) {
  return Math.hypot(a.x - b.x, a.y - b.y)
}

// eyeAspectRatio menghitung EAR dari 6 titik kontur satu mata (urutan sesuai
// hasil landmarks.getLeftEye()/getRightEye() milik face-api.js, yaitu p1..p6
// mengelilingi mata sesuai konvensi 68-point landmark model).
function eyeAspectRatio(eye) {
  const [p1, p2, p3, p4, p5, p6] = eye
  return (dist(p2, p6) + dist(p3, p5)) / (2 * dist(p1, p4))
}

let modelsLoadPromise = null
function loadModels() {
  if (!modelsLoadPromise) {
    modelsLoadPromise = Promise.all([
      faceapi.nets.tinyFaceDetector.loadFromUri('/models'),
      faceapi.nets.faceLandmark68Net.loadFromUri('/models'),
    ])
  }
  return modelsLoadPromise
}

// -- Deteksi wajah tahan cahaya redup --
// Laporan pegawai: kedipan mata HAMPIR SELALU tidak terdeteksi saat absen
// PULANG (berbeda dengan absen masuk yang biasanya lancar). Akar masalahnya
// bukan di perhitungan EAR/kedipan di atas, tapi SEBELUM itu: deteksi wajah
// oleh TinyFaceDetector sendiri gagal (faceDetected tidak pernah true) pada
// kondisi cahaya yang lebih redup/backlit yang umum terjadi sore/malam hari
// saat pulang -- kalau wajah tidak pernah terdeteksi, EAR tidak pernah bisa
// dihitung sama sekali, jadi kedipan juga tidak akan pernah terdeteksi,
// berapa pun bagus & jelasnya pegawai benar-benar berkedip.
//
// Dua perbaikan di sini menyasar akar masalah itu (bukan cuma menurunkan
// ambang EAR yang sudah adaptif di atas):
// 1. Setiap frame digambar dulu ke <canvas> tersembunyi dengan filter
//    kecerahan/kontras SEBELUM dideteksi (bukan langsung dari <video>) --
//    teknik umum untuk membantu deteksi wajah pada gambar under-exposed.
//    Kecerahan makin dinaikkan bertahap kalau wajah belum juga terdeteksi.
// 2. scoreThreshold TinyFaceDetector juga ikut diturunkan bertahap kalau
//    wajah belum terdeteksi cukup lama, supaya deteksi lebih "toleran" pada
//    kondisi sulit tanpa mengorbankan akurasi saat cahaya sudah cukup
//    (tahap 0 = pengaturan normal, tidak berubah sama sekali).
const NO_FACE_STAGE_MS = [0, 1000, 3000] // mulai tahap 1 setelah 1s, tahap 2 setelah 3s
const STAGE_BRIGHTNESS = [1, 1.35, 1.75]
const STAGE_SCORE_THRESHOLD = [0.4, 0.3, 0.2]

export function useBlinkLiveness() {
  const modelsReady = ref(false)
  const modelsError = ref('')
  const faceDetected = ref(false)
  const blinkDetected = ref(false)
  const currentEar = ref(null)
  // lowLight: true kalau wajah belum terdeteksi cukup lama (tahap >= 1) --
  // dipakai AbsensiView.vue untuk menyarankan pegawai pindah ke tempat yang
  // lebih terang, bukan cuma diam menunggu tanpa penjelasan.
  const lowLight = ref(false)

  let intervalId = null
  let videoEl = null
  let wasClosed = false
  let closedSince = 0
  let ticking = false
  let baselineEar = null
  // earHistory: 1 sampel EAR mentah sebelumnya, dipakai untuk smoothing
  // sederhana (rata-rata 2 sampel) supaya noise deteksi landmark satu frame
  // (lebih sering terjadi di cahaya redup) tidak salah terbaca sebagai
  // kedipan atau, sebaliknya, menutupi kedipan yang sesungguhnya.
  let prevRawEar = null
  let lastFaceAt = Date.now()
  let workCanvas = null
  let workCtx = null

  async function init() {
    if (modelsReady.value) return
    try {
      await loadModels()
      modelsReady.value = true
    } catch (e) {
      modelsError.value = 'Gagal memuat model deteksi wajah: ' + (e?.message || e)
      throw e
    }
  }

  function reset() {
    blinkDetected.value = false
    faceDetected.value = false
    currentEar.value = null
    lowLight.value = false
    wasClosed = false
    closedSince = 0
    baselineEar = null
    prevRawEar = null
    lastFaceAt = Date.now()
  }

  // ensureWorkCanvas menyiapkan <canvas> kerja seukuran frame video saat ini
  // (dibuat sekali, dipakai ulang tiap tick -- resize otomatis kalau ukuran
  // video berubah, mis. baru selesai load metadata).
  function ensureWorkCanvas(video) {
    const w = video.videoWidth || 320
    const h = video.videoHeight || 240
    if (!workCanvas) {
      workCanvas = document.createElement('canvas')
      workCtx = workCanvas.getContext('2d')
    }
    if (workCanvas.width !== w || workCanvas.height !== h) {
      workCanvas.width = w
      workCanvas.height = h
    }
    return workCanvas
  }

  async function tick() {
    if (ticking || !videoEl || videoEl.readyState < 2) return
    ticking = true
    try {
      const noFaceMs = Date.now() - lastFaceAt
      const stage = noFaceMs >= NO_FACE_STAGE_MS[2] ? 2 : noFaceMs >= NO_FACE_STAGE_MS[1] ? 1 : 0
      lowLight.value = stage >= 1

      // gambar frame ke canvas kerja dulu (opsional filter kecerahan/kontras
      // kalau sedang di tahap toleransi cahaya redup) -- dideteksi dari
      // canvas ini, bukan langsung dari elemen <video>.
      const canvas = ensureWorkCanvas(videoEl)
      workCtx.filter = stage > 0 ? `brightness(${STAGE_BRIGHTNESS[stage]}) contrast(1.1)` : 'none'
      workCtx.drawImage(videoEl, 0, 0, canvas.width, canvas.height)

      // inputSize 320 (naik dari 224) & scoreThreshold diturunkan bertahap
      // kalau wajah belum terdeteksi cukup lama -- supaya wajah tetap
      // terdeteksi pada kondisi cahaya lebih redup (mis. absen pulang sore/
      // malam hari) tanpa mengorbankan akurasi saat cahaya sudah cukup
      // (tahap 0 dipakai selama wajah masih normal terdeteksi).
      const result = await faceapi
        .detectSingleFace(canvas, new faceapi.TinyFaceDetectorOptions({ inputSize: 320, scoreThreshold: STAGE_SCORE_THRESHOLD[stage] }))
        .withFaceLandmarks(true)
      if (!result) {
        faceDetected.value = false
        currentEar.value = null
        return
      }
      lastFaceAt = Date.now()
      lowLight.value = false
      faceDetected.value = true
      const leftEar = eyeAspectRatio(result.landmarks.getLeftEye())
      const rightEar = eyeAspectRatio(result.landmarks.getRightEye())
      const rawEar = (leftEar + rightEar) / 2
      // smoothing 2-sampel supaya noise per-frame tidak memicu/menutupi
      // kedipan secara keliru.
      const ear = prevRawEar == null ? rawEar : (rawEar + prevRawEar) / 2
      prevRawEar = rawEar
      currentEar.value = ear

      if (baselineEar == null) {
        // Bootstrap: anggap frame pertama yang berhasil terdeteksi adalah
        // kondisi mata terbuka normal (paling mungkin benar -- pegawai baru
        // menghadapkan wajah ke kamera, belum sempat berkedip).
        baselineEar = clampBaseline(ear)
      }

      const now = Date.now()
      const openThreshold = baselineEar * EAR_OPEN_RATIO
      const closedThreshold = baselineEar * EAR_CLOSED_RATIO

      if (ear <= closedThreshold) {
        if (!wasClosed) {
          wasClosed = true
          closedSince = now
        }
      } else if (ear >= openThreshold) {
        if (wasClosed && now - closedSince < MAX_CLOSED_MS) {
          blinkDetected.value = true
        }
        wasClosed = false
        // Baseline hanya diperbarui saat mata dianggap terbuka (bukan saat
        // tertutup/di zona abu-abu di antaranya), supaya baseline tidak
        // ikut turun akibat kedipan itu sendiri, dan tetap mengikuti
        // perubahan cahaya/posisi yang lambat selama sesi absen.
        baselineEar = clampBaseline(baselineEar * (1 - BASELINE_EMA_ALPHA) + ear * BASELINE_EMA_ALPHA)
      }
      // ear di antara closedThreshold..openThreshold: zona abu-abu (transisi
      // menutup/membuka) -- sengaja tidak mengubah status apa pun di sini.
    } catch {
      // deteksi gagal sesaat (mis. frame video belum benar-benar siap) --
      // abaikan, akan dicoba lagi pada tick berikutnya.
    } finally {
      ticking = false
    }
  }

  function start(video) {
    videoEl = video
    reset()
    if (intervalId) clearInterval(intervalId)
    intervalId = setInterval(tick, 200)
  }

  function stop() {
    if (intervalId) clearInterval(intervalId)
    intervalId = null
    videoEl = null
  }

  return { modelsReady, modelsError, faceDetected, blinkDetected, currentEar, lowLight, init, start, stop, reset }
}
