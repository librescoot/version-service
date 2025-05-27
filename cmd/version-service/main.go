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

func main() {
	// Parse command line arguments
	redisAddr := flag.String("redis", "192.168.7.1:6379", "Redis server address")
	hashName := flag.String("hash", "os-release", "Redis hash name to store the values")
	flag.Parse()

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

	// Read and store serial number if available
	serialNumber, err := readSerialNumber()
	if err != nil {
		log.Printf("Warning: Failed to read serial number: %v", err)
	} else if serialNumber != "" {
		err = rdb.HSet(ctx, *hashName, "serial_number", serialNumber).Err()
		if err != nil {
			log.Fatalf("Failed to set serial number in Redis: %v", err)
		}
		log.Printf("Successfully stored serial number in Redis hash '%s'", *hashName)
	}

	// Read and store real serial number if available
	serialNumberReal, err := readSerialNumberReal()
	if err != nil {
		log.Printf("Warning: Failed to read real serial number: %v", err)
	} else if serialNumberReal != "" {
		err = rdb.HSet(ctx, *hashName, "serial_number_real", serialNumberReal).Err()
		if err != nil {
			log.Fatalf("Failed to set real serial number in Redis: %v", err)
		}
		log.Printf("Successfully stored real serial number in Redis hash '%s'", *hashName)
	}

	log.Printf("Successfully stored OS release information in Redis hash '%s'", *hashName)
}

// readSerialNumber reads the component serial number based on the hash name
// It attempts to read the serial number from the system files
func readSerialNumber() (string, error) {
	cfg0Str, err := getRawOTPValue("/sys/fsl_otp/HW_OCOTP_CFG0", 4)
	if err != nil {
		return "", fmt.Errorf("failed to get raw OTP value for CFG0: %v", err)
	}
	cfg1Str, err := getRawOTPValue("/sys/fsl_otp/HW_OCOTP_CFG1", 8)
	if err != nil {
		return "", fmt.Errorf("failed to get raw OTP value for CFG1: %v", err)
	}

	cfg0, err := parseHexFromString(cfg0Str)
	if err != nil {
		return "", fmt.Errorf("failed to parse serial number part 1 (CFG0 from '%s'): %v", cfg0Str, err)
	}
	cfg1, err := parseHexFromString(cfg1Str)
	if err != nil {
		return "", fmt.Errorf("failed to parse serial number part 2 (CFG1 from '%s'): %v", cfg1Str, err)
	}

	// Combine the values
	sn := cfg0 + cfg1
	return fmt.Sprintf("%d", sn), nil
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
	hexStr := fmt.Sprintf("%02x%02x%02x%02x", buffer[0], buffer[1], buffer[2], buffer[3])

	if len(hexStr) != 8 {
		return "", fmt.Errorf("internal error: formatted hex string length is not 8: got '%s'", hexStr)
	}
	return hexStr, nil
}

// getRawOTPValue attempts to read a raw hex string from a sysfs file,
// falling back to NVMEM if the file doesn't exist or is unreadable.
// It returns the hex string without "0x" prefix (e.g., "00112233").
func getRawOTPValue(filePath string, nvmemOffset int) (string, error) {
	data, err := os.ReadFile(filePath)
	if err == nil {
		content := strings.TrimSpace(string(data))
		if strings.HasPrefix(strings.ToLower(content), "0x") {
			return content[2:], nil
		}
		return content, nil
	}

	log.Printf("Warning: Failed to read from %s (%v), attempting fallback to NVMEM for offset %d", filePath, err, nvmemOffset)
	nvmemValue, nvmemErr := readHexValueFromNvmem(nvmemOffset)
	if nvmemErr != nil {
		return "", fmt.Errorf("failed to read from %s (err: %v) and NVMEM fallback also failed for offset %d (err: %v)", filePath, err, nvmemOffset, nvmemErr)
	}
	return nvmemValue, nil
}

// parseHexFromString parses a hexadecimal string (expected without "0x" prefix) into a uint64.
func parseHexFromString(hexStr string) (uint64, error) {
	value, err := strconv.ParseUint(hexStr, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("cannot parse hex string '%s': %v", hexStr, err)
	}
	return value, nil
}

// readSerialNumberReal reads the component serial number as a concatenated string.
// It constructs the chip serial number the "real" (correct) way, falling back to NVMEM.
func readSerialNumberReal() (string, error) {
	uidHStr, err := getRawOTPValue("/sys/fsl_otp/HW_OCOTP_CFG1", 8)
	if err != nil {
		return "", fmt.Errorf("failed to get raw OTP value for real serial number part H (CFG1): %v", err)
	}

	uidLStr, err := getRawOTPValue("/sys/fsl_otp/HW_OCOTP_CFG0", 4)
	if err != nil {
		return "", fmt.Errorf("failed to get raw OTP value for real serial number part L (CFG0): %v", err)
	}

	// Combine the values as strings
	serialNumberReal := uidHStr + uidLStr
	return serialNumberReal, nil
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
