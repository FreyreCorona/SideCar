package core

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"
)

// ACFGenerator provides ACF generation via Wine + AHMISimGenDemo.exe.
// Requires distrobox with winebox container and the extracted AppImage binaries.
type ACFGenerator struct {
	// GenDir is the path to the AHMISimGenDemo_og.exe directory inside distrobox.
	// Default: ~/ahmi-work/Gen/
	GenDir string
	// DistroboxName is the name of the distrobox container. Default: "winebox".
	DistroboxName string
	// PlatformCode: 0=GC9002 (240x240), 1=GC9003, 2=GC9005. Default: 0.
	PlatformCode int
	// DitherMode: 0=none, 1=ordered. Default: 1.
	DitherMode int
}

// DefaultACFGenerator returns a generator with default settings.
func DefaultACFGenerator() *ACFGenerator {
	home, _ := os.UserHomeDir()
	return &ACFGenerator{
		GenDir:        filepath.Join(home, "ahmi-work", "Gen"),
		DistroboxName: "winebox",
		PlatformCode:  0, // GC9002 (240x240)
		DitherMode:    1,
	}
}

// GenerateResult contains the output paths from ACF generation.
type GenerateResult struct {
	// TextureACF is the path to the generated Texture.acf file.
	TextureACF string
	// ConfigDataACF is the path to the generated ConfigData&Texture.acf file.
	ConfigDataACF string
	// OutputDir is the directory where ACF files were generated.
	OutputDir string
}

// GenerateFromZip generates an ACF from file.zip using Wine + AHMISimGenDemo.exe.
// The file.zip should be placed in the GenDir before calling this function,
// or use ReplaceImageInZip to modify it first.
func (g *ACFGenerator) GenerateFromZip(ctx context.Context, zipPath string, outDir string) (*GenerateResult, error) {
	if g.GenDir == "" {
		return nil, fmt.Errorf("GenDir is required")
	}
	if g.DistroboxName == "" {
		g.DistroboxName = "winebox"
	}

	// Ensure output directory exists
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create output dir: %w", err)
	}

	// Copy file.zip to GenDir if not already there
	genDirZip := filepath.Join(g.GenDir, "file.zip")
	zipAbs, err := filepath.Abs(zipPath)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve zip path: %w", err)
	}
	if zipAbs != genDirZip {
		if err := copyFile(zipAbs, genDirZip); err != nil {
			return nil, fmt.Errorf("cannot copy file.zip to GenDir: %w", err)
		}
	}

	// Build the Wine command
	// Note: paths inside distrobox are relative to home (~/ahmi-work/Gen/)
	// We need to use the distrobox path format
	genDirRel, _ := filepath.Rel(filepath.Join(os.Getenv("HOME")), g.GenDir)
	outDirAbs, _ := filepath.Abs(outDir)
	outDirRel, _ := filepath.Rel(filepath.Join(os.Getenv("HOME")), outDirAbs)

	// Use distrobox enter with bash -c to run the command
	script := fmt.Sprintf(`cd ~/%s && rm -rf json/ && echo "13" | WINEDEBUG=-all wine AHMISimGenDemo_og.exe -f file.zip -m 2 -c 0 -e %d -d %d -o ~/%s 2>&1`,
		genDirRel, g.PlatformCode, g.DitherMode, outDirRel)

	cmd := exec.CommandContext(ctx, "distrobox", "enter", g.DistroboxName, "--", "bash", "-c", script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ACF generation failed: %w\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	// Find generated ACF files
	textureACF := filepath.Join(outDir, "Texture.acf")
	configDataACF := filepath.Join(outDir, "ConfigData&Texture.acf")

	// Check if files were generated
	if _, err := os.Stat(textureACF); os.IsNotExist(err) {
		return nil, fmt.Errorf("Texture.acf not generated in %s", outDir)
	}

	return &GenerateResult{
		TextureACF:    textureACF,
		ConfigDataACF: configDataACF,
		OutputDir:     outDir,
	}, nil
}

// GenerateFromImage generates an ACF from a single image file by:
// 1. Loading the original file.zip
// 2. Replacing the target image(s) with the provided image
// 3. Generating ACF from the modified zip
func (g *ACFGenerator) GenerateFromImage(ctx context.Context, imagePath string, outDir string, targetFilenames ...string) (*GenerateResult, error) {
	// Load the source image
	srcFile, err := os.Open(imagePath)
	if err != nil {
		return nil, fmt.Errorf("cannot open image: %w", err)
	}
	defer srcFile.Close()

	srcData, err := io.ReadAll(srcFile)
	if err != nil {
		return nil, fmt.Errorf("cannot read image: %w", err)
	}

	// Create a temp file.zip with replaced images
	tmpZip, err := os.CreateTemp("", "sidecar-acf-*.zip")
	if err != nil {
		return nil, fmt.Errorf("cannot create temp zip: %w", err)
	}
	defer os.Remove(tmpZip.Name())
	defer tmpZip.Close()

	// Load original file.zip from GenDir
	origZipPath := filepath.Join(g.GenDir, "file.zip")
	if err := ReplaceImagesInZip(origZipPath, tmpZip.Name(), srcData, targetFilenames...); err != nil {
		return nil, fmt.Errorf("cannot replace images in zip: %w", err)
	}

	return g.GenerateFromZip(ctx, tmpZip.Name(), outDir)
}

// ReplaceImagesInZip replaces files in a zip archive with new data.
// If targetFilenames is empty, replaces all PNG/GIF files.
func ReplaceImagesInZip(srcZipPath, dstZipPath string, newData []byte, targetFilenames ...string) error {
	srcZip, err := zip.OpenReader(srcZipPath)
	if err != nil {
		return fmt.Errorf("cannot open source zip: %w", err)
	}
	defer srcZip.Close()

	dstFile, err := os.Create(dstZipPath)
	if err != nil {
		return fmt.Errorf("cannot create destination zip: %w", err)
	}
	defer dstFile.Close()

	w := zip.NewWriter(dstFile)
	defer w.Close()

	for _, file := range srcZip.File {
		// Check if this is a target file to replace
		shouldReplace := false
		if len(targetFilenames) == 0 {
			// Replace all PNG/GIF files
			ext := filepath.Ext(file.Name)
			if ext == ".png" || ext == ".gif" {
				shouldReplace = true
			}
		} else {
			if slices.Contains(targetFilenames, file.Name) {
				shouldReplace = true
			}
		}

		if shouldReplace {
			// Write new data
			fw, err := w.Create(file.Name)
			if err != nil {
				return fmt.Errorf("cannot create entry %s: %w", file.Name, err)
			}
			if _, err := fw.Write(newData); err != nil {
				return fmt.Errorf("cannot write entry %s: %w", file.Name, err)
			}
		} else {
			// Copy original data
			fw, err := w.Create(file.Name)
			if err != nil {
				return fmt.Errorf("cannot create entry %s: %w", file.Name, err)
			}
			rc, err := file.Open()
			if err != nil {
				return fmt.Errorf("cannot open entry %s: %w", file.Name, err)
			}
			if _, err := io.Copy(fw, rc); err != nil {
				rc.Close()
				return fmt.Errorf("cannot copy entry %s: %w", file.Name, err)
			}
			rc.Close()
		}
	}

	return nil
}

// CheckWineAvailable checks if the Wine/distrobox setup is available.
func (g *ACFGenerator) CheckWineAvailable() error {
	// Check if distrobox is available
	if _, err := exec.LookPath("distrobox"); err != nil {
		return fmt.Errorf("distrobox not found in PATH: %w", err)
	}

	// Check if the container exists
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "distrobox", "list")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cannot list distrobox containers: %w", err)
	}
	if !bytes.Contains(output, []byte(g.DistroboxName)) {
		return fmt.Errorf("distrobox container %q not found", g.DistroboxName)
	}

	// Check if GenDir exists
	if _, err := os.Stat(g.GenDir); os.IsNotExist(err) {
		return fmt.Errorf("GenDir %q does not exist", g.GenDir)
	}

	// Check if exe exists
	exePath := filepath.Join(g.GenDir, "AHMISimGenDemo_og.exe")
	if _, err := os.Stat(exePath); os.IsNotExist(err) {
		return fmt.Errorf("AHMISimGenDemo_og.exe not found in %s", g.GenDir)
	}

	return nil
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
