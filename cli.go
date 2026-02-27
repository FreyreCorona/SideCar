package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/FreyreCorona/SideCar/core"
)

func runCLI(args []string) error {
	fs := flag.NewFlagSet("sidecar", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	// Flags globales
	device := fs.String("device", "auto", "device path or 'auto'")
	baud := fs.Uint("baud", 115200, "baud rate")
	cmd := fs.String("cmd", "", "command to run (required)")
	help := fs.Bool("help", false, "display command help")

	// Flags por comando
	brightness := fs.Int("brightness", 100, "brightness level 0–255 [cmd: on, brightness]")
	filePath := fs.String("file", "", "path to file to upload [cmd: upload]")
	fileType := fs.String("type", "texture", "file type: texture, texture_gif, cpu1, bootloader, ... [cmd: upload]")
	regID := fs.Uint("reg", 0, "register ID (uint16) [cmd: write-reg, read-reg]")
	regVal := fs.Uint("val", 0, "register value (uint32) [cmd: write-reg]")
	regStr := fs.String("str", "", "string value [cmd: write-str]")
	pageID := fs.Int("page", 1, "page number to display [cmd: show-page]")
	regs := fs.String("regs", "", "registers as \"ID1=VAL1,ID2=VAL2,...\" [cmd: write-regs]")

	if err := fs.Parse(args[1:]); err != nil {
		return fmt.Errorf("error parsing flags: %v\n\n%s", err, usage())
	}

	if *help || *cmd == "" {
		if *cmd != "" {
			fmt.Println(cmdHelp(*cmd))
		} else {
			fmt.Println(usage())
		}
		return nil
	}

	// ── Conexión ─────────────────────────────────────────────────────────────

	dev, err := connect(*device, int(*baud))
	if err != nil {
		return err
	}
	defer dev.Close()
	dev.SetLogger(os.Stdout)

	// ── Dispatch ──────────────────────────────────────────────────────────────

	switch *cmd {

	case "on":
		if *brightness < 0 || *brightness > 255 {
			return fmt.Errorf("-brightness debe estar entre 0 y 255")
		}
		return dev.SetBrightness(uint8(*brightness))

	case "off":
		return dev.Sleep()

	case "brightness":
		if *brightness < 0 || *brightness > 255 {
			return fmt.Errorf("-brightness debe estar entre 0 y 255")
		}
		return dev.SetBrightness(uint8(*brightness))

	case "reboot":
		return dev.Reboot()

	case "upload":
		if *filePath == "" {
			return fmt.Errorf("upload requiere -file <path>")
		}

		data, err := os.ReadFile(*filePath)
		if err != nil {
			return fmt.Errorf("no se puede leer %s: %w", *filePath, err)
		}

		cfg := core.UploadConfig{
			OnProgress: func(p core.UploadProgress) {
				fmt.Printf("\r  uploading... %d%%", p.Percent)
				if p.Percent == 100 {
					fmt.Println()
				}
			},
		}

		if err := dev.UploadFile(data, core.FileType(*fileType), cfg); err != nil {
			return fmt.Errorf("upload falló: %w", err)
		}

		fmt.Printf("✓ archivo subido (%s)\n", *fileType)
		return nil

	case "write-reg":
		if *regID > 0xFFFF {
			return fmt.Errorf("-reg debe ser un uint16 válido")
		}
		result, err := dev.WriteNumRegisters([]core.NumRegister{
			{RegID: uint16(*regID), Value: uint32(*regVal)},
		})
		if err != nil {
			return fmt.Errorf("write-reg falló: %w", err)
		}
		for id, val := range result {
			fmt.Printf("✓ reg[0x%04X] = %d\n", id, val)
		}
		return nil

	case "read-reg":
		if *regID > 0xFFFF {
			return fmt.Errorf("-reg debe ser un uint16 válido")
		}
		result, err := dev.ReadNumRegisters([]uint16{uint16(*regID)})
		if err != nil {
			return fmt.Errorf("read-reg falló: %w", err)
		}
		for id, val := range result {
			fmt.Printf("reg[0x%04X] = %d (0x%08X)\n", id, val, val)
		}
		return nil

	case "write-str":
		if *regID > 0xFFFF {
			return fmt.Errorf("-reg debe ser un uint16 válido")
		}
		if *regStr == "" {
			return fmt.Errorf("write-str requiere -str <valor>")
		}
		data := []byte(*regStr)
		_, err := dev.WriteStringRegister(uint16(*regID), data)
		if err != nil {
			return fmt.Errorf("write-str falló: %w", err)
		}
		fmt.Printf("✓ reg[0x%04X] = %q\n", *regID, *regStr)
		return nil

	case "read-str":
		if *regID > 0xFFFF {
			return fmt.Errorf("-reg debe ser un uint16 válido")
		}
		data, err := dev.ReadStringRegister(uint16(*regID), 64)
		if err != nil {
			return fmt.Errorf("read-str falló: %w", err)
		}
		fmt.Printf("reg[0x%04X] = %q (%s)\n", *regID, string(data), hex.EncodeToString(data))
		return nil

	case "show-page":
		if *pageID < 1 {
			return fmt.Errorf("-page debe ser >= 1")
		}
		result, err := dev.WriteNumRegisters([]core.NumRegister{
			{RegID: core.RegCurrentPage, Value: uint32(*pageID)},
		})
		if err != nil {
			return fmt.Errorf("show-page falló: %w", err)
		}
		for _, val := range result {
			fmt.Printf("✓ página activa = %d\n", val)
		}
		return nil

	case "write-regs":
		if *regs == "" {
			return fmt.Errorf("write-regs requiere -regs \"ID1=VAL1,ID2=VAL2,...\"")
		}
		parsedRegs, err := parseRegs(*regs)
		if err != nil {
			return err
		}
		result, err := dev.WriteNumRegisters(parsedRegs)
		if err != nil {
			return fmt.Errorf("write-regs falló: %w", err)
		}
		for id, val := range result {
			fmt.Printf("✓ reg[0x%04X] = %d\n", id, val)
		}
		return nil

	default:
		return fmt.Errorf("comando desconocido: %q\n\n%s", *cmd, usage())
	}
}

// pageRegID y regBrightness usan las constantes definidas en core/tags.go.
// '当前页面序号' → RegCurrentPage = 2  (no 1, confirmado en tagUtils.js)
// '背光'         → RegBrightness  = 7

// ── Helpers ────────────────────────────────────────────────────────────────────

func connect(device string, baud int) (*core.Device, error) {
	if device != "auto" {
		sp, err := core.OpenSerial(device, baud)
		if err != nil {
			return nil, fmt.Errorf("no se puede abrir %s a %d baud: %w", device, baud, err)
		}
		dev := core.NewDevice(sp)
		if _, err := dev.Handshake(); err != nil {
			dev.Close()
			return nil, fmt.Errorf("handshake fallido en %s: %w", device, err)
		}
		return dev, nil
	}

	dev, err := core.AutoConnect(baud)
	if err != nil {
		return nil, fmt.Errorf("no se encontró ningún dispositivo compatible a %d baud: %w", baud, err)
	}
	return dev, nil
}

func parseRegs(input string) ([]core.NumRegister, error) {
	parts := strings.Split(input, ",")
	regs := make([]core.NumRegister, 0, len(parts))
	for _, p := range parts {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("formato inválido %q, use ID=VALOR", p)
		}
		id, err := strconv.ParseUint(strings.TrimSpace(kv[0]), 0, 16)
		if err != nil {
			return nil, fmt.Errorf("regID inválido %q: %w", kv[0], err)
		}
		val, err := strconv.ParseUint(strings.TrimSpace(kv[1]), 0, 32)
		if err != nil {
			return nil, fmt.Errorf("valor inválido %q: %w", kv[1], err)
		}
		regs = append(regs, core.NumRegister{RegID: uint16(id), Value: uint32(val)})
	}
	return regs, nil
}

// ── Help strings ──────────────────────────────────────────────────────────────

func usage() string {
	return `Sidecar CLI — controla una Minitela por serial

Uso:
  sidecar -cmd <comando> [opciones]

Comandos:
  on            Enciende la pantalla (brillo por defecto 100)
  off           Apaga la pantalla
  brightness    Ajusta el brillo sin cambiar el estado
  reboot        Reinicia el dispositivo
  upload        Sube un archivo de firmware o textura
  write-reg     Escribe un registro numérico
  write-regs    Escribe múltiples registros en un batch
  read-reg      Lee un registro numérico
  write-str     Escribe un registro de tipo string
  read-str      Lee un registro de tipo string
  show-page     Cambia la página activa en la pantalla

Opciones globales:
  -device string   ruta del puerto serial o 'auto' (default "auto")
  -baud uint       baud rate (default 115200)
  -help            muestra ayuda; combinado con -cmd muestra ayuda del comando

Ejemplos:
  sidecar -cmd on
  sidecar -cmd on -brightness 80
  sidecar -cmd off -device /dev/ttyUSB0
  sidecar -cmd upload -file Texture.acf -type texture
  sidecar -cmd write-reg -reg 0x0001 -val 3
  sidecar -cmd write-str -reg 0x0010 -str "Hola mundo"
  sidecar -cmd show-page -page 2
  sidecar -cmd on -help
`
}

func cmdHelp(cmd string) string {
	helps := map[string]string{
		"on": `Comando: on
Enciende la pantalla ajustando el brillo.

Opciones:
  -brightness int   nivel de brillo 0–255 (default 100)

Ejemplos:
  sidecar -cmd on
  sidecar -cmd on -brightness 200
`,
		"off": `Comando: off
Apaga la pantalla (brillo = 0).

Ejemplo:
  sidecar -cmd off
`,
		"brightness": `Comando: brightness
Ajusta el brillo sin modificar el estado de la pantalla.

Opciones:
  -brightness int   nivel de brillo 0–255 (requerido)

Ejemplo:
  sidecar -cmd brightness -brightness 150
`,
		"upload": `Comando: upload
Sube un archivo binario (.acf) al dispositivo. Soporta reanudación automática.

Flujo para imágenes y GIFs:
  1. Genera el .acf con AHMISimGenDemo.exe (incluido en IDE_utils/Gen/)
       AHMISimGenDemo.exe -f file.zip -m 2 -c 0 -e -o ./ACF
  2. Sube el archivo generado:
       sidecar -cmd upload -file Texture.acf  -type texture
       sidecar -cmd upload -file GIF_0.acf    -type texture_gif

Opciones:
  -file string   ruta al archivo .acf (requerido)
  -type string   destino en el dispositivo (default "texture"):
                   texture       → 0x08100000  imagen/textura principal
                   texture_gif   → 0x08500000  GIF animado
                   cpu1          → firmware principal
                   cpu0_config   → configuración CPU0
                   bootloader    → bootloader (precaución)

Ejemplos:
  sidecar -cmd upload -file Texture.acf -type texture
  sidecar -cmd upload -file GIF_0.acf -type texture_gif
`,
		"write-regs": `Comando: write-regs
Escribe múltiples registros numéricos en un solo batch (más eficiente que write-reg repetido).

Opciones:
  -regs string   lista de registros en formato "ID1=VAL1,ID2=VAL2,..." (requerido)
                 Los IDs pueden ser decimales o hex (0x...)

Ejemplo:
  sidecar -cmd write-regs -regs "1080=75,1081=40,1082=90"
  sidecar -cmd write-regs -regs "0x0438=75,0x0439=40"
`,
		"write-reg": `Comando: write-reg
Escribe un valor numérico (uint32) en un registro del dispositivo.

Opciones:
  -reg uint   ID del registro en hex o decimal (requerido)
  -val uint   valor a escribir (default 0)

Ejemplos:
  sidecar -cmd write-reg -reg 0x0001 -val 3
  sidecar -cmd write-reg -reg 7 -val 100
`,
		"read-reg": `Comando: read-reg
Lee el valor numérico de un registro.

Opciones:
  -reg uint   ID del registro (requerido)

Ejemplo:
  sidecar -cmd read-reg -reg 0x0001
`,
		"write-str": `Comando: write-str
Escribe un valor string en un registro del dispositivo.

Opciones:
  -reg uint      ID del registro (requerido)
  -str string    texto a escribir (requerido)

Ejemplo:
  sidecar -cmd write-str -reg 0x0010 -str "22°C"
`,
		"show-page": `Comando: show-page
Cambia la página activa en la pantalla.

Opciones:
  -page int   número de página (default 1)

Ejemplo:
  sidecar -cmd show-page -page 3
`,
	}

	if h, ok := helps[cmd]; ok {
		return h
	}
	return fmt.Sprintf("No hay ayuda disponible para %q\n\n%s", cmd, usage())
}
