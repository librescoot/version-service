package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
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

	// Store data in Redis hash
	for key, value := range osReleaseData {
		err = rdb.HSet(ctx, *hashName, key, value).Err()
		if err != nil {
			log.Fatalf("Failed to set Redis hash field %s: %v", key, err)
		}
	}

	log.Printf("Successfully stored OS release information in Redis hash '%s'", *hashName)
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
