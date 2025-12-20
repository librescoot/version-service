package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

var version = "dev"

// getIdentifierHexStrings attempts to read raw hex strings for CFG0 and CFG1.
// It prioritizes NVMEM, then falls back to OTP sysfs files.
// Returns the hex strings (which may be empty if a part is unreadable) and an error if any part could not be read from any source.
func getIdentifierHexStrings() (cfg0Hex string, cfg1Hex string, err error) {
	nvmemDevicePath := "/sys/bus/nvmem/devices/imx-ocotp0/nvmem"
	otpCfg0Path := "/sys/fsl_otp/HW_OCOTP_CFG0"
	otpCfg1Path := "/sys/fsl_otp/HW_OCOTP_CFG1"

	nvmemPresent := false
	if _, statErr := os.Stat(nvmemDevicePath); statErr == nil {
		nvmemPresent = true
	}

	var errMessages []string

	// --- Read CFG0 (Unique ID Part L) ---
	var cfg0ErrDetails []string
	// Try NVMEM for CFG0
	if nvmemPresent {
		val, nvmemErr := readHexValueFromNvmem(4) // Offset 4 for CFG0
		if nvmemErr == nil {
			cfg0Hex = val
		} else {
			// NVMEM read failed, will try OTP. Store error detail.
			cfg0ErrDetails = append(cfg0ErrDetails, fmt.Sprintf("NVMEM(offset 4): %s", nvmemErr.Error()))
		}
	} else {
		cfg0ErrDetails = append(cfg0ErrDetails, "NVMEM: not found")
	}

	// If CFG0 not successfully read from NVMEM, try OTP
	if cfg0Hex == "" {
		data, otpErr := os.ReadFile(otpCfg0Path)
		if otpErr == nil {
			content := strings.TrimSpace(string(data))
			cfg0Hex = strings.TrimPrefix(strings.ToLower(content), "0x")
			// If OTP succeeded, previous NVMEM error details for CFG0 are irrelevant for this part's success
			cfg0ErrDetails = []string{}
		} else {
			cfg0ErrDetails = append(cfg0ErrDetails, fmt.Sprintf("OTP(%s): %s", otpCfg0Path, otpErr.Error()))
		}
	}
	// If CFG0 is still empty after trying all sources, record the failure.
	if cfg0Hex == "" && len(cfg0ErrDetails) > 0 {
		errMessages = append(errMessages, fmt.Sprintf("CFG0_read_failed: {%s}", strings.Join(cfg0ErrDetails, ", ")))
	}

	// --- Read CFG1 (Unique ID Part H) ---
	var cfg1ErrDetails []string
	// Try NVMEM for CFG1
	if nvmemPresent {
		val, nvmemErr := readHexValueFromNvmem(8) // Offset 8 for CFG1
		if nvmemErr == nil {
			cfg1Hex = val
		} else {
			cfg1ErrDetails = append(cfg1ErrDetails, fmt.Sprintf("NVMEM(offset 8): %s", nvmemErr.Error()))
		}
	} else {
		cfg1ErrDetails = append(cfg1ErrDetails, "NVMEM: not found")
	}

	// If CFG1 not successfully read from NVMEM, try OTP
	if cfg1Hex == "" {
		data, otpErr := os.ReadFile(otpCfg1Path)
		if otpErr == nil {
			content := strings.TrimSpace(string(data))
			cfg1Hex = strings.TrimPrefix(strings.ToLower(content), "0x")
			cfg1ErrDetails = []string{}
		} else {
			cfg1ErrDetails = append(cfg1ErrDetails, fmt.Sprintf("OTP(%s): %s", otpCfg1Path, otpErr.Error()))
		}
	}
	if cfg1Hex == "" && len(cfg1ErrDetails) > 0 {
		errMessages = append(errMessages, fmt.Sprintf("CFG1_read_failed: {%s}", strings.Join(cfg1ErrDetails, ", ")))
	}

	if len(errMessages) > 0 {
		err = fmt.Errorf(strings.Join(errMessages, "; "))
	}
	return
}

func main() {
	// Parse command line arguments
	redisAddr := flag.String("redis", "192.168.7.1:6379", "Redis server address")
	hashName := flag.String("hash", "os-release", "Redis hash name to store the values")
	flag.Parse()

	log.Printf("librescoot-version %s starting", version)

	// Read /etc/os-release file
	osReleaseData, err := readOSRelease()
	if err != nil {
		log.Fatalf("Failed to read OS release information: %v", err)
	}

	// Connect to Redis
	rdb := redis.NewClient(&redis.Options{
		Addr: *redisAddr,
	})
	defer rdb.Close()

	ctx := context.Background()

	// Check Redis connection
	_, err = rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Failed to connect to Redis at %s: %v", *redisAddr, err)
	}

	// Store OS release data in Redis hash
	for key, value := range osReleaseData {
		err = rdb.HSet(ctx, *hashName, key, value).Err()
		if err != nil {
			log.Fatalf("Failed to set Redis hash field %s: %v", key, err)
		}
	}
	log.Printf("Successfully stored OS release information in Redis hash '%s'", *hashName)

	// Read device identifier parts (CFG0, CFG1)
	cfg0Hex, cfg1Hex, partsErr := getIdentifierHexStrings()

	if partsErr != nil {
		log.Printf("Warning: Failed to read one or more device identifier parts: %v", partsErr)
	}

	// Process and store "legacy" serial number (CFG0 + CFG1 as uint64)
	if cfg0Hex != "" && cfg1Hex != "" {
		cfg0Val, errParse0 := parseHexFromString(cfg0Hex)
		cfg1Val, errParse1 := parseHexFromString(cfg1Hex)

		if errParse0 == nil && errParse1 == nil {
			legacySN := cfg0Val + cfg1Val
			err = rdb.HSet(ctx, *hashName, "serial_number", fmt.Sprintf("%d", legacySN)).Err()
			if err != nil {
				// Use Fatalf for critical Redis errors to prevent partial state
				log.Fatalf("Failed to set legacy serial number in Redis: %v", err)
			}
			log.Printf("Successfully stored legacy serial number in Redis hash '%s'", *hashName)
		} else {
			var legacySnErrParts []string
			if errParse0 != nil {
				legacySnErrParts = append(legacySnErrParts, fmt.Sprintf("CFG0 ('%s') parse error: %v", cfg0Hex, errParse0))
			}
			if errParse1 != nil {
				legacySnErrParts = append(legacySnErrParts, fmt.Sprintf("CFG1 ('%s') parse error: %v", cfg1Hex, errParse1))
			}
			log.Printf("Warning: Failed to calculate legacy serial number: %s", strings.Join(legacySnErrParts, "; "))
		}
	} else if partsErr == nil { // Only log this if partsErr didn't already cover the missing parts
		log.Printf("Warning: Could not calculate legacy serial number because one or both identifier parts (CFG0, CFG1) are missing.")
	}

	// Process and store "real" serial number (CFG1_hex_string + CFG0_hex_string)
	if cfg0Hex != "" && cfg1Hex != "" {
		realSN := cfg1Hex + cfg0Hex // Concatenation of hex strings
		err = rdb.HSet(ctx, *hashName, "serial_number_real", realSN).Err()
		if err != nil {
			log.Fatalf("Failed to set real serial number in Redis: %v", err)
		}
		log.Printf("Successfully stored real serial number in Redis hash '%s'", *hashName)
	} else if partsErr == nil { // Only log this if partsErr didn't already cover the missing parts
		log.Printf("Warning: Could not store real serial number because one or both identifier parts (CFG0, CFG1) are missing.")
	}
}

// readHexValueFromNvmem reads a 4-byte hex value from NVMEM at a given offset.
// It returns an 8-character hex string.
func readHexValueFromNvmem(offset int) (string, error) {
	nvmemDevicePath := "/sys/bus/nvmem/devices/imx-ocotp0/nvmem"

	file, err := os.Open(nvmemDevicePath)
	if err != nil {
		return "", fmt.Errorf("failed to open NVMEM device %s: %v", nvmemDevicePath, err)
	}
	defer file.Close()

	_, err = file.Seek(int64(offset), 0) // 0 means relative to the start of the file
	if err != nil {
		return "", fmt.Errorf("failed to seek in NVMEM device %s to offset %d: %v", nvmemDevicePath, offset, err)
	}

	buffer := make([]byte, 4)
	n, err := file.Read(buffer)
	if err != nil {
		return "", fmt.Errorf("failed to read from NVMEM device %s at offset %d: %v", nvmemDevicePath, offset, err)
	}
	if n != 4 {
		return "", fmt.Errorf("unexpected number of bytes read from NVMEM device %s at offset %d: got %d, expected 4", nvmemDevicePath, offset, n)
	}

	// Format the 4 bytes read from NVMEM into an 8-character hexadecimal string.
	// To emulate `hexdump -e '1/4 "%08x\n"'` on a little-endian system,
	// the bytes B0, B1, B2, B3 should be formatted as B3B2B1B0.
	hexStr := fmt.Sprintf("%02x%02x%02x%02x", buffer[3], buffer[2], buffer[1], buffer[0])

	if len(hexStr) != 8 {
		return "", fmt.Errorf("internal error: formatted hex string length is not 8: got '%s'", hexStr)
	}
	return hexStr, nil
}

// parseHexFromString parses a hexadecimal string (expected without "0x" prefix) into a uint64.
func parseHexFromString(hexStr string) (uint64, error) {
	value, err := strconv.ParseUint(hexStr, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("cannot parse hex string '%s': %v", hexStr, err)
	}
	return value, nil
}

// readOSRelease reads the /etc/os-release file and returns a map of lowercase keys to values
func readOSRelease() (map[string]string, error) {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return nil, fmt.Errorf("failed to open /etc/os-release: %w", err)
	}
	defer file.Close()

	data := make(map[string]string)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.ToLower(parts[0])
		value := strings.Trim(parts[1], "\"")
		data[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading /etc/os-release: %w", err)
	}

	return data, nil
}
