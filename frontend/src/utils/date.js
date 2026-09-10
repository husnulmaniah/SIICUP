// toApiDate formats a JS Date as "YYYY-MM-DD" using its LOCAL calendar date
// (year/month/day) -- NEVER via `date.toISOString().slice(0, 10)`.
//
// A date picked from a PrimeVue DatePicker/Calendar is a Date object set to
// *local* midnight of the picked day. `toISOString()` first converts that
// instant to UTC, which for any positive UTC offset timezone (WIB = UTC+7,
// WITA = UTC+8, WIT = UTC+9 -- i.e. all of Indonesia) rolls local midnight
// back into the *previous* day in UTC. The result: picking "11 September"
// silently gets sent to the backend (and saved) as "10 September". This bit
// both the Pengajuan Cuti date range and every date field on the generic
// CrudManager (Tanggal Merah, TMT, dsb).
//
// Reading the Date object's own local getFullYear()/getMonth()/getDate()
// instead avoids the UTC round-trip entirely, so the date sent to the API is
// always exactly the day the user picked/typed, regardless of timezone.
export function toApiDate(d) {
  if (!(d instanceof Date) || Number.isNaN(d.getTime())) return d
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}
