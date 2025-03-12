package utils

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strings"
)

// CompressFile compresse un fichier en utilisant gzip
func CompressFile(filePath string) error {
	// Ouvrir le fichier source
	source, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer source.Close()

	// Créer le fichier de destination
	target := filePath + ".gz"
	destination, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destination.Close()

	// Créer un writer gzip
	gzipWriter := gzip.NewWriter(destination)
	defer gzipWriter.Close()

	// Copier les données
	_, err = io.Copy(gzipWriter, source)
	if err != nil {
		return fmt.Errorf("failed to compress file: %w", err)
	}

	// Fermer le writer gzip pour s'assurer que toutes les données sont écrites
	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}

	// Supprimer le fichier original
	if err := os.Remove(filePath); err != nil {
		// Si la suppression échoue, on essaie de supprimer le fichier compressé
		os.Remove(target)
		return fmt.Errorf("failed to remove original file: %w", err)
	}

	return nil
}

// DecompressFile décompresse un fichier gzip
func DecompressFile(filePath string) error {
	// Vérifier que le fichier est un fichier gzip
	if !strings.HasSuffix(filePath, ".gz") {
		return fmt.Errorf("file is not a gzip file: %s", filePath)
	}

	// Ouvrir le fichier source
	source, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer source.Close()

	// Créer un reader gzip
	gzipReader, err := gzip.NewReader(source)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	// Créer le fichier de destination
	target := strings.TrimSuffix(filePath, ".gz")
	destination, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destination.Close()

	// Copier les données
	_, err = io.Copy(destination, gzipReader)
	if err != nil {
		return fmt.Errorf("failed to decompress file: %w", err)
	}

	// Supprimer le fichier compressé
	if err := os.Remove(filePath); err != nil {
		// Si la suppression échoue, on essaie de supprimer le fichier décompressé
		os.Remove(target)
		return fmt.Errorf("failed to remove compressed file: %w", err)
	}

	return nil
}
