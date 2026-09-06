package api

import (
	"encoding/hex"
	"fmt"
	"net/http"

	"strings"

	"nofx/logger"
	"nofx/mcp/payment"
	"nofx/wallet"

	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/gin-gonic/gin"
)

type beginnerOnboardingResponse struct {
	Address           string `json:"address"`
	PrivateKey        string `json:"private_key"`
	Chain             string `json:"chain"`
	Asset             string `json:"asset"`
	Provider          string `json:"provider"`
	DefaultModel      string `json:"default_model"`
	ConfiguredModelID string `json:"configured_model_id"`
	BalanceUSDC       string `json:"balance_usdc"`
	BalanceStatus     string `json:"balance_status,omitempty"`
	EnvSaved          bool   `json:"env_saved"`
	EnvPath           string `json:"env_path,omitempty"`
	ReusedExisting    bool   `json:"reused_existing"`
	EnvWarning        string `json:"env_warning,omitempty"`
}

type currentBeginnerWalletResponse struct {
	Found         bool   `json:"found"`
	Address       string `json:"address,omitempty"`
	BalanceUSDC   string `json:"balance_usdc,omitempty"`
	BalanceStatus string `json:"balance_status,omitempty"`
	Source        string `json:"source,omitempty"`
	Claw402Status string `json:"claw402_status"`
}

// queryBeginnerWalletBalance returns the wallet balance plus a status flag so
// the UI can distinguish "RPC unreachable" from a genuinely empty wallet.
// Uses the 30s cache — safe for UI polling.
func queryBeginnerWalletBalance(address string) (balanceUSDC string, balanceStatus string) {
	balance, err := wallet.QueryUSDCBalanceCached(address)
	if err != nil {
		return "", "unknown"
	}
	return fmt.Sprintf("%.2f", balance), "ok"
}

func (s *Server) handleBeginnerOnboarding(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing user context"})
		return
	}

	privateKey, address, configuredModelID, reusedExisting, err := s.resolveBeginnerWallet(userID)
	if err != nil {
		logger.Errorf("Failed to resolve beginner wallet for user %s: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare beginner wallet"})
		return
	}

	if !reusedExisting {
		if err := s.store.AIModel().Update(userID, "claw402", true, privateKey, "", payment.DefaultClaw402Model); err != nil {
			logger.Errorf("Failed to save beginner claw402 config for user %s: %v", userID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save beginner model configuration"})
			return
		}

		configuredModelID, err = s.findConfiguredClaw402ModelID(userID)
		if err != nil {
			logger.Warnf("Could not resolve configured claw402 model id for user %s: %v", userID, err)
		}
	}

	balanceUSDC, balanceStatus := queryBeginnerWalletBalance(address)
	resp := beginnerOnboardingResponse{
		Address:           address,
		PrivateKey:        privateKey,
		Chain:             "base",
		Asset:             "USDC",
		Provider:          "claw402",
		DefaultModel:      payment.DefaultClaw402Model,
		ConfiguredModelID: configuredModelID,
		BalanceUSDC:       balanceUSDC,
		BalanceStatus:     balanceStatus,
		ReusedExisting:    reusedExisting,
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) handleCurrentBeginnerWallet(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing user context"})
		return
	}
	claw402Status := checkClaw402Health()

	models, err := s.store.AIModel().List(userID)
	if err != nil {
		logger.Errorf("Failed to load current beginner wallet for user %s: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load current wallet"})
		return
	}

	for _, model := range models {
		if model == nil || model.Provider != "claw402" {
			continue
		}

		privateKey := strings.TrimSpace(model.APIKey.String())
		if privateKey == "" {
			continue
		}

		address, addrErr := walletAddressFromPrivateKey(privateKey)
		if addrErr != nil {
			logger.Warnf("Failed to derive current beginner wallet for user %s: %v", userID, addrErr)
			continue
		}

		balanceUSDC, balanceStatus := queryBeginnerWalletBalance(address)
		c.JSON(http.StatusOK, currentBeginnerWalletResponse{
			Found:         true,
			Address:       address,
			BalanceUSDC:   balanceUSDC,
			BalanceStatus: balanceStatus,
			Source:        "model",
			Claw402Status: claw402Status,
		})
		return
	}

	c.JSON(http.StatusOK, currentBeginnerWalletResponse{
		Found:         false,
		Claw402Status: claw402Status,
	})
}

func (s *Server) resolveBeginnerWallet(userID string) (privateKey string, address string, configuredModelID string, reused bool, err error) {
	// 1. Check if current user already has a claw402 wallet
	models, err := s.store.AIModel().List(userID)
	if err != nil {
		return "", "", "", false, err
	}

	for _, model := range models {
		if model == nil || model.Provider != "claw402" {
			continue
		}
		existingKey := strings.TrimSpace(model.APIKey.String())
		if existingKey == "" {
			continue
		}

		addr, addrErr := walletAddressFromPrivateKey(existingKey)
		if addrErr != nil {
			logger.Warnf("Existing claw402 key for user %s is invalid, regenerating: %v", userID, addrErr)
			break
		}

		return existingKey, addr, model.ID, true, nil
	}

	privateKeyObj, genErr := gethcrypto.GenerateKey()
	if genErr != nil {
		return "", "", "", false, genErr
	}

	addr := gethcrypto.PubkeyToAddress(privateKeyObj.PublicKey)
	keyHex := "0x" + hex.EncodeToString(gethcrypto.FromECDSA(privateKeyObj))
	return keyHex, addr.Hex(), "", false, nil
}

func (s *Server) findConfiguredClaw402ModelID(userID string) (string, error) {
	models, err := s.store.AIModel().List(userID)
	if err != nil {
		return "", err
	}

	for _, model := range models {
		if model != nil && model.Provider == "claw402" {
			return model.ID, nil
		}
	}

	return "", fmt.Errorf("claw402 model not found")
}

func walletAddressFromPrivateKey(privateKey string) (string, error) {
	key := strings.TrimSpace(privateKey)
	if !strings.HasPrefix(key, "0x") {
		return "", fmt.Errorf("private key must start with 0x")
	}
	if len(key) != 66 {
		return "", fmt.Errorf("private key must be 66 characters")
	}

	privateKeyObj, err := gethcrypto.HexToECDSA(strings.TrimPrefix(key, "0x"))
	if err != nil {
		return "", err
	}

	return gethcrypto.PubkeyToAddress(privateKeyObj.PublicKey).Hex(), nil
}
