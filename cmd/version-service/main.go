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
	"time"

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
	if nvmemPresent {
		val, nvmemErr := readHexValueFromNvmem(4) // Offset 4 for CFG0
		if nvmemErr == nil {
			cfg0Hex = val
		} else {
			cfg0ErrDetails = append(cfg0ErrDetails, fmt.Sprintf("NVMEM(offset 4): %s", nvmemErr.Error()))
		}
	} else {
		cfg0ErrDetails = append(cfg0ErrDetails, "NVMEM: not found")
	}

	if cfg0Hex == "" {
		data, otpErr := os.ReadFile(otpCfg0Path)
		if otpErr == nil {
			content := strings.TrimSpace(string(data))
			cfg0Hex = strings.TrimPrefix(strings.ToLower(content), "0x")
			cfg0ErrDetails = []string{}
		} else {
			cfg0ErrDetails = append(cfg0ErrDetails, fmt.Sprintf("OTP(%s): %s", otpCfg0Path, otpErr.Error()))
		}
	}
	if cfg0Hex == "" && len(cfg0ErrDetails) > 0 {
		errMessages = append(errMessages, fmt.Sprintf("CFG0_read_failed: {%s}", strings.Join(cfg0ErrDetails, ", ")))
	}

	// --- Read CFG1 (Unique ID Part H) ---
	var cfg1ErrDetails []string
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
	redisAddr := flag.String("redis", "192.168.7.1:6379", "Redis server address")
	hashName := flag.String("hash", "os-release", "Redis hash name to store the values")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("version-service %s\n", version)
		return
	}

	if os.Getenv("JOURNAL_STREAM") != "" {
		log.SetFlags(0)
	} else {
		log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	}

	log.Printf("librescoot-version %s starting", version)

	osReleaseData, err := readOSRelease()
	if err != nil {
		log.Fatalf("Failed to read OS release information: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:         *redisAddr,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})
	defer rdb.Close()

	ctx := context.Background()

	_, err = rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Failed to connect to Redis at %s: %v", *redisAddr, err)
	}

	fields := make(map[string]interface{}, len(osReleaseData)+2)
	for key, value := range osReleaseData {
		fields[key] = value
	}

	// Read device identifier parts (CFG0, CFG1)
	cfg0Hex, cfg1Hex, partsErr := getIdentifierHexStrings()

	if partsErr != nil {
		log.Printf("Warning: Failed to read one or more device identifier parts: %v", partsErr)
	}

	if cfg0Hex != "" && cfg1Hex != "" {
		cfg0Val, errParse0 := parseHexFromString(cfg0Hex)
		cfg1Val, errParse1 := parseHexFromString(cfg1Hex)

		if errParse0 == nil && errParse1 == nil {
			fields["serial_number"] = fmt.Sprintf("%d", cfg0Val+cfg1Val)
			fields["serial_number_real"] = cfg1Hex + cfg0Hex
		} else {
			var parseErrParts []string
			if errParse0 != nil {
				parseErrParts = append(parseErrParts, fmt.Sprintf("CFG0 ('%s') parse error: %v", cfg0Hex, errParse0))
			}
			if errParse1 != nil {
				parseErrParts = append(parseErrParts, fmt.Sprintf("CFG1 ('%s') parse error: %v", cfg1Hex, errParse1))
			}
			log.Printf("Warning: Failed to calculate serial numbers: %s", strings.Join(parseErrParts, "; "))
		}
	} else if partsErr != nil {
		log.Printf("Warning: Could not compute serial numbers, identifier parts missing")
	}

	// Write all fields in a single Redis call
	if err := rdb.HSet(ctx, *hashName, fields).Err(); err != nil {
		log.Fatalf("Failed to write to Redis hash '%s': %v", *hashName, err)
	}

	log.Printf("Stored %d fields in Redis hash '%s'", len(fields), *hashName)
}

// readHexValueFromNvmem reads a 4-byte hex value from NVMEM at a given offset.
func readHexValueFromNvmem(offset int) (string, error) {
	nvmemDevicePath := "/sys/bus/nvmem/devices/imx-ocotp0/nvmem"

	file, err := os.Open(nvmemDevicePath)
	if err != nil {
		return "", fmt.Errorf("failed to open NVMEM device %s: %v", nvmemDevicePath, err)
	}
	defer file.Close()

	_, err = file.Seek(int64(offset), 0)
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

	hexStr := fmt.Sprintf("%02x%02x%02x%02x", buffer[3], buffer[2], buffer[1], buffer[0])
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
