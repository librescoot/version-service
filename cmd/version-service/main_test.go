package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A healthy slot: /boot/zImage is the image's symlink to zImage-<release> and the
// matching module tree is there.
func TestCheckKernelHealthy(t *testing.T) {
	root := t.TempDir()
	const release = "6.12.34-fslc-test"
	mustMkdir(t, filepath.Join(root, "lib/modules", release))
	mustMkdir(t, filepath.Join(root, "boot"))
	mustSymlink(t, "zImage-"+release, filepath.Join(root, "boot/zImage"))

	fields := checkKernel(root, release)
	if fields["kernel_check"] != "ok" {
		t.Fatalf("kernel_check = %q (%s)", fields["kernel_check"], fields["kernel_check_detail"])
	}
	if fields["kernel_modules"] != "ok" || fields["kernel_boot_image"] != "ok" {
		t.Fatalf("fields = %v", fields)
	}
	if fields["kernel_release"] != release {
		t.Fatalf("kernel_release = %q", fields["kernel_release"])
	}
}

func TestCheckKernelSkew(t *testing.T) {
	const release = "6.12.34-fslc-test"
	tests := []struct {
		name       string
		modules    string // module tree to create, empty for none
		bootImage  func(t *testing.T, path string)
		wantCheck  string
		wantModule string
		wantImage  string
	}{
		{
			name:       "module tree missing",
			bootImage:  func(t *testing.T, p string) { mustSymlink(t, "zImage-"+release, p) },
			wantCheck:  "skew",
			wantModule: "missing",
			wantImage:  "ok",
		},
		{
			name:       "boot image points at another release",
			modules:    release,
			bootImage:  func(t *testing.T, p string) { mustSymlink(t, "zImage-6.12.34-fslc-old", p) },
			wantCheck:  "skew",
			wantModule: "ok",
			wantImage:  "mismatch",
		},
		{
			name:    "boot image replaced by a plain file",
			modules: release,
			bootImage: func(t *testing.T, p string) {
				if err := os.WriteFile(p, []byte("kernel"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantCheck:  "skew",
			wantModule: "ok",
			wantImage:  "not-symlink",
		},
		{
			name:       "boot image missing",
			modules:    release,
			bootImage:  func(t *testing.T, p string) {},
			wantCheck:  "skew",
			wantModule: "ok",
			wantImage:  "missing",
		},
		{
			name:    "both wrong",
			modules: "",
			bootImage: func(t *testing.T, p string) {
				if err := os.WriteFile(p, []byte("kernel"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantCheck:  "skew",
			wantModule: "missing",
			wantImage:  "not-symlink",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			mustMkdir(t, filepath.Join(root, "boot"))
			if tt.modules != "" {
				mustMkdir(t, filepath.Join(root, "lib/modules", tt.modules))
			}
			tt.bootImage(t, filepath.Join(root, "boot/zImage"))

			fields := checkKernel(root, release)
			if fields["kernel_check"] != tt.wantCheck {
				t.Errorf("kernel_check = %q, want %q", fields["kernel_check"], tt.wantCheck)
			}
			if fields["kernel_modules"] != tt.wantModule {
				t.Errorf("kernel_modules = %q, want %q", fields["kernel_modules"], tt.wantModule)
			}
			if fields["kernel_boot_image"] != tt.wantImage {
				t.Errorf("kernel_boot_image = %q, want %q", fields["kernel_boot_image"], tt.wantImage)
			}
			if tt.wantCheck == "skew" && fields["kernel_check_detail"] == "" {
				t.Error("skew without a detail line")
			}
		})
	}
}

// Without a release there is nothing to compare against, and guessing would
// report skew on a board that is fine.
func TestCheckKernelWithoutRelease(t *testing.T) {
	fields := checkKernel(t.TempDir(), "")
	if fields["kernel_check"] != "unknown" {
		t.Fatalf("kernel_check = %q, want unknown", fields["kernel_check"])
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustSymlink(t *testing.T, target, path string) {
	t.Helper()
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}
