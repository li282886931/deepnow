// main.go - 主创建入口
// by szyok
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"time"
)

type ServerConfig struct {
	EndpointIP    string `json:"endpoint_ip"`
	EndpointPort  string `json:"endpoint_port"`
	WebUIIP       string `json:"webui_ip"`
	WebUIPort     string `json:"webui_port"`
	DomainName    string `json:"domain_name"`
	MaxTotalConns int    `json:"max_total_conns"`
	MaxConnsPerIP int    `json:"max_conns_per_ip"`
	CertFile      string `json:"cert_file"`
	KeyFile       string `json:"key_file"`
}

var config ServerConfig
var configFile = "aibase_server_config.json"

func loadConfig() error {
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		config = ServerConfig{
			EndpointIP: "0.0.0.0", EndpointPort: "8443",
			WebUIIP: "0.0.0.0", WebUIPort: "8080",
			DomainName: "", MaxTotalConns: 1000, MaxConnsPerIP: 50,
			CertFile: "server.crt", KeyFile: "server.key",
		}
		return saveConfig()
	} else {
		file, err := os.ReadFile(configFile)
		if err != nil {
			return fmt.Errorf("读取配置失败: %w", err)
		}
		if err := json.Unmarshal(file, &config); err != nil {
			return fmt.Errorf("解析配置失败: %w", err)
		}
		needSave := false
		if config.CertFile == "" {
			config.CertFile = "server.crt"
			needSave = true
		}
		if config.KeyFile == "" {
			config.KeyFile = "server.key"
			needSave = true
		}
		if needSave {
			return saveConfig()
		}
	}
	return nil
}

func saveConfig() error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("编码配置失败: %w", err)
	}
	if err := os.WriteFile(configFile, data, 0644); err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}
	return nil
}

func ensureCert() error {
	if _, err := os.Stat(config.CertFile); os.IsNotExist(err) {
		fmt.Println("🛡️ 未检测到外部 HTTPS 证书，正在生成兜底的高强度自签名证书...")
		priv, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return fmt.Errorf("生成私钥失败: %w", err)
		}
		host := config.DomainName
		if host == "" {
			host = "localhost"
		}
		template := x509.Certificate{
			SerialNumber: big.NewInt(1),
			Subject:      pkix.Name{Organization: []string{"AI Base Server"}},
			DNSNames:     []string{host},
			NotBefore:    time.Now(), NotAfter: time.Now().Add(3650 * 24 * time.Hour),
			KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
			ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			BasicConstraintsValid: true,
		}
		derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
		if err != nil {
			return fmt.Errorf("生成证书失败: %w", err)
		}
		certOut, err := os.Create(config.CertFile)
		if err != nil {
			return fmt.Errorf("创建证书文件失败: %w", err)
		}
		if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
			certOut.Close()
			return fmt.Errorf("写入证书失败: %w", err)
		}
		if err := certOut.Close(); err != nil {
			return fmt.Errorf("关闭证书文件失败: %w", err)
		}
		keyOut, err := os.Create(config.KeyFile)
		if err != nil {
			return fmt.Errorf("创建私钥文件失败: %w", err)
		}
		if err := pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}); err != nil {
			keyOut.Close()
			return fmt.Errorf("写入私钥失败: %w", err)
		}
		if err := keyOut.Close(); err != nil {
			return fmt.Errorf("关闭私钥文件失败: %w", err)
		}
		fmt.Println("✅ 默认自签名证书生成完毕！")
	} else if err != nil {
		return fmt.Errorf("检查证书文件失败: %w", err)
	}
	return nil
}

func main() {
	fmt.Println("==================================================")
	fmt.Println("🚀 AI Base Server (Deepnow) (算力聚合基座) 启动中...")
	fmt.Println("==================================================")
	if err := loadConfig(); err != nil {
		log.Fatal(err)
	}
	if err := ensureCert(); err != nil {
		log.Fatal(err)
	}
	if err := initDatabases(); err != nil {
		log.Fatal(err)
	}

	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/chat/completions", handleGatewayCompletions)
		addr := fmt.Sprintf("%s:%s", config.EndpointIP, config.EndpointPort)
		fmt.Printf("🔒 Endpoint (HTTPS) -> https://%s/v1/chat/completions\n", addr)
		srv := &http.Server{Addr: addr, Handler: mux, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
		log.Fatal(srv.ListenAndServeTLS(config.CertFile, config.KeyFile))
	}()

	muxWeb := http.NewServeMux()
	muxWeb.HandleFunc("/api/system/build", requireAdmin(handleGetBuildInfo))
	muxWeb.HandleFunc("/api/system/status", requireAdmin(handleGetSystemStatus))
	muxWeb.HandleFunc("/api/login", handleAdminLogin)
	muxWeb.HandleFunc("/api/logout", handleAdminLogout)
	muxWeb.HandleFunc("/api/session", requireAdmin(handleAdminSession))
	muxWeb.HandleFunc("/api/config/get", requireAdmin(handleGetConfig))
	muxWeb.HandleFunc("/api/config/save", requireAdmin(handleSaveConfig))
	muxWeb.HandleFunc("/api/keys/generate", requireAdmin(handleGenerateKeys))
	muxWeb.HandleFunc("/api/keys/list", requireAdmin(handleListKeys))
	muxWeb.HandleFunc("/api/keys/import", requireAdmin(handleImportKeys))
	muxWeb.HandleFunc("/api/keys/delete", requireAdmin(handleDeleteKeys))
	muxWeb.HandleFunc("/api/keys/usage", requireAdmin(handleKeyUsage))
	muxWeb.HandleFunc("/api/models/get", requireAdmin(handleGetModelConfigs))
	muxWeb.HandleFunc("/api/models/save", requireAdmin(handleSaveModelConfig))
	muxWeb.HandleFunc("/api/models/activate", requireAdmin(handleActivateModel))
	muxWeb.HandleFunc("/api/models/test", requireAdmin(handleModelTest))
	muxWeb.HandleFunc("/api/rag/upload", requireAdmin(handleRAGUpload))
	muxWeb.HandleFunc("/api/rag/status", requireAdmin(handleRAGStatus))
	muxWeb.HandleFunc("/api/rag/documents", requireAdmin(handleRAGDocuments))
	muxWeb.HandleFunc("/api/rag/documents/", requireAdmin(handleDeleteRAGDocument))
	muxWeb.HandleFunc("/api/rag/test", requireAdmin(handleRAGTest))
	muxWeb.HandleFunc("/api/diagnostics", requireAdmin(handleDiagnostics))
	muxWeb.HandleFunc("/api/audit/events", requireAdmin(handleAuditEvents))
	muxWeb.HandleFunc("/api/rules/get", requireAdmin(handleGetRules))
	muxWeb.HandleFunc("/api/rules/save", requireAdmin(handleSaveRules))
	muxWeb.HandleFunc("/api/test/chat", requireAdmin(handleTestChat))
	muxWeb.Handle("/", embeddedFrontendHandler())

	webAddr := fmt.Sprintf("%s:%s", config.WebUIIP, config.WebUIPort)
	fmt.Printf("🟣 WebUI 管理后台 (HTTP) -> http://%s\n", webAddr)
	log.Fatal(http.ListenAndServe(webAddr, muxWeb))
}
