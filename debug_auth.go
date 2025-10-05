package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

func getAdminPassword() string {
	if password := os.Getenv("ADMIN_PASSWORD"); password != "" {
		return password
	}
	return "admin"
}

func debugAuth() {
	currentPassword := getAdminPassword()
	hashedPassword := hashPassword(currentPassword)

	fmt.Printf("当前密码: %s\n", currentPassword)
	fmt.Printf("哈希后的密码: %s\n", hashedPassword)

	// 测试验证
	testPassword := "admin"
	testHashed := hashPassword(testPassword)
	fmt.Printf("测试密码: %s\n", testPassword)
	fmt.Printf("测试哈希: %s\n", testHashed)
	fmt.Printf("验证结果: %t\n", hashedPassword == testHashed)
}
