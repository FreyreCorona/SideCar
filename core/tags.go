package core

// ─────────────────────────────────────────────
// Registros del sistema (systemTagNameMap en tagUtils.js)
// ─────────────────────────────────────────────

const (
	RegDate        = uint16(4)  // Fecha en formato numérico 0xYYYYMMDD
	RegTime        = uint16(5)  // Hora en formato numérico 0xHHMMSS
	RegBrightness  = uint16(7)  // Brillo de la pantalla 0–255
	RegCPU0Version = uint16(12) // Versión CPU0 (string)
	RegCPU1Version = uint16(13) // Versión CPU1 (string)
	RegCurrentPage = uint16(2)  // Página activa en la pantalla
)

// ─────────────────────────────────────────────
// Registros de datos (tagNameMap en tagUtils.js)
// ─────────────────────────────────────────────

const (
	// Sistema / hardware
	RegCPUUsage       = uint16(1080)
	RegGPUUsage       = uint16(1081)
	RegBatteryPercent = uint16(1082)
	RegWifiSSID       = uint16(1083) // string
	RegWifiQuality    = uint16(1084) // 0=sin señal, 1=baja, 2=media, 3=alta
	RegBTName         = uint16(1085) // string
	RegBTStatus       = uint16(1086) // 0=desconectado, 1=conectado
	RegWifiStatus     = uint16(1087) // 0=desconectado, 1=conectado
	RegBatteryType    = uint16(1150) // 0–5 según nivel (0=<20%, 5=100%)

	// Recordatorios
	RegReminder1Content = uint16(1090) // string
	RegReminder1Time    = uint16(1091) // string
	RegReminder2Content = uint16(1092) // string
	RegReminder2Time    = uint16(1093) // string
	RegReminder3Content = uint16(1094) // string
	RegReminder3Time    = uint16(1095) // string

	// Media
	RegMediaName     = uint16(1100) // string
	RegMediaDuration = uint16(1101)
	RegMediaNow      = uint16(1102)
	RegMediaPlay     = uint16(2003) // 0=parado, 1=reproduciendo

	// Clima (5 días)
	RegWeather1Type    = uint16(1110)
	RegWeather1Temp    = uint16(1111)
	RegWeather1TempMin = uint16(1112)
	RegWeather1TempMax = uint16(1113)
	RegWeather1Desc    = uint16(1114) // string

	RegWeather2Type    = uint16(1115)
	RegWeather2Temp    = uint16(1116)
	RegWeather2TempMin = uint16(1117)
	RegWeather2TempMax = uint16(1118)
	RegWeather2Desc    = uint16(1119) // string

	RegWeather3Type    = uint16(1120)
	RegWeather3Temp    = uint16(1121)
	RegWeather3TempMin = uint16(1122)
	RegWeather3TempMax = uint16(1123)
	RegWeather3Desc    = uint16(1124) // string

	RegWeather4Type    = uint16(1125)
	RegWeather4Temp    = uint16(1126)
	RegWeather4TempMin = uint16(1127)
	RegWeather4TempMax = uint16(1128)
	RegWeather4Desc    = uint16(1129) // string

	RegWeather5Type    = uint16(1130)
	RegWeather5Temp    = uint16(1131)
	RegWeather5TempMin = uint16(1132)
	RegWeather5TempMax = uint16(1133)
	RegWeather5Desc    = uint16(1134) // string

	// Notificaciones
	RegNotif1Title   = uint16(1140) // string
	RegNotif1Content = uint16(1141) // string
	RegNotif2Title   = uint16(1142) // string
	RegNotif2Content = uint16(1143) // string
	RegNotif3Title   = uint16(1144) // string
	RegNotif3Content = uint16(1145) // string
)

// ─────────────────────────────────────────────
// WifiQuality convierte RSSI/porcentaje al enum que espera la pantalla.
// Misma lógica que tagUtils.js
// ─────────────────────────────────────────────

func WifiQualityLevel(qualityPct int) uint32 {
	switch {
	case qualityPct <= 30:
		return 0
	case qualityPct <= 50:
		return 1
	case qualityPct < 70:
		return 2
	default:
		return 3
	}
}

// BatteryLevel convierte un porcentaje de batería al enum de icono.
// Misma lógica que tagUtils.js
func BatteryLevel(percent int) uint32 {
	switch {
	case percent < 20:
		return 0
	case percent < 40:
		return 1
	case percent < 60:
		return 2
	case percent < 80:
		return 3
	case percent < 100:
		return 4
	default:
		return 5
	}
}

// WeatherType convierte el nombre del tipo de clima al enum de icono.
// Misma lógica que WeatherTypes en tagUtils.js
func WeatherType(main string) uint32 {
	switch main {
	case "Clear":
		return 1
	case "Clouds", "Mist", "Smoke", "Haze", "Dust", "Fog", "Sand", "Ash", "Squall", "Tornado":
		return 2
	case "Thunderstorm", "Drizzle", "Rain", "Snow":
		return 4
	default:
		return 0
	}
}
