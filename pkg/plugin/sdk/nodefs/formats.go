package nodefs

// Hand-written helpers shared by the panel-side consumers of this module;
// API names follow the panel's lower_snake archive format identifiers.

var archiveFormatAPINames = map[ArchiveFormat]string{
	ArchiveFormat_ARCHIVE_FORMAT_ZIP:      "zip",
	ArchiveFormat_ARCHIVE_FORMAT_TAR:      "tar",
	ArchiveFormat_ARCHIVE_FORMAT_TAR_GZ:   "tar_gz",
	ArchiveFormat_ARCHIVE_FORMAT_TAR_BZ2:  "tar_bz2",
	ArchiveFormat_ARCHIVE_FORMAT_TAR_XZ:   "tar_xz",
	ArchiveFormat_ARCHIVE_FORMAT_TAR_ZSTD: "tar_zstd",
	ArchiveFormat_ARCHIVE_FORMAT_GZ:       "gz",
	ArchiveFormat_ARCHIVE_FORMAT_BZ2:      "bz2",
	ArchiveFormat_ARCHIVE_FORMAT_XZ:       "xz",
	ArchiveFormat_ARCHIVE_FORMAT_ZSTD:     "zstd",
	ArchiveFormat_ARCHIVE_FORMAT_7Z:       "7z",
	ArchiveFormat_ARCHIVE_FORMAT_RAR:      "rar",
}

var archiveFormatsByAPIName = func() map[string]ArchiveFormat {
	m := make(map[string]ArchiveFormat, len(archiveFormatAPINames))
	for format, name := range archiveFormatAPINames {
		m[name] = format
	}

	return m
}()

// ArchiveFormatToAPIName maps a format onto its lower_snake API name;
// unspecified/unknown formats map to "".
func ArchiveFormatToAPIName(format ArchiveFormat) string {
	return archiveFormatAPINames[format]
}

// ArchiveFormatFromAPIName resolves a lower_snake API name; unknown names
// map to ARCHIVE_FORMAT_UNSPECIFIED.
func ArchiveFormatFromAPIName(name string) ArchiveFormat {
	return archiveFormatsByAPIName[name]
}
