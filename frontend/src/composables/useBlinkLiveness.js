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

// Ambang batas EAR -- nilai umum yang dipakai pada implementasi blink
// detection berbasis EAR (mata dianggap "tertutup" di bawah ~0.21, "terbuka
// normal" di atas ~0.26; ada gap di antaranya supaya tidak flip-flop akibat
// noise deteksi).
const EAR_CLOSED = 0.21
const EAR_OPEN = 0.26
// Kedipan wajar berlangsung < 400ms, tapi deteksi kita hanya sampling tiap
// ~200ms jadi kita beri toleransi jendela waktu tertutup->terbuka sampai 1.5s
// (mencakup jeda antar-sampling + kedipan yang agak lambat).
const MAX_CLOSED_MS = 1500

export function useBlinkLiveness() {
  const modelsReady = ref(false)
  const modelsError = ref('')
  const faceDetected = ref(false)
  const blinkDetected = ref(false)
  const currentEar = ref(null)

  let intervalId = null
  let videoEl = null
  let wasClosed = false
  let closedSince = 0
  let ticking = false

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
    wasClosed = false
    closedSince = 0
  }

  async function tick() {
    if (ticking || !videoEl || videoEl.readyState < 2) return
    ticking = true
    try {
      const result = await faceapi
        .detectSingleFace(videoEl, new faceapi.TinyFaceDetectorOptions({ inputSize: 224, scoreThreshold: 0.5 }))
        .withFaceLandmarks(true)
      if (!result) {
        faceDetected.value = false
        currentEar.value = null
        return
      }
      faceDetected.value = true
      const leftEar = eyeAspectRatio(result.landmarks.getLeftEye())
      const rightEar = eyeAspectRatio(result.landmarks.getRightEye())
      const ear = (leftEar + rightEar) / 2
      currentEar.value = ear

      const now = Date.now()
      if (ear < EAR_CLOSED) {
        if (!wasClosed) {
          wasClosed = true
          closedSince = now
        }
      } else if (ear > EAR_OPEN) {
        if (wasClosed && now - closedSince < MAX_CLOSED_MS) {
          blinkDetected.value = true
        }
        wasClosed = false
      }
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

  return { modelsReady, modelsError, faceDetected, blinkDetected, currentEar, init, start, stop, reset }
}
