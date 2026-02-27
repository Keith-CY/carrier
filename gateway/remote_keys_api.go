package gateway

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const remoteKeyUploadRequestMaxBytes int64 = remoteKeyUploadMaxBytes + 64*1024

func handleRemoteKeys(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	flags := effectiveGatewayFeatureFlags(cfg)
	if !flags.RemoteControlPlaneEnabled {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "remote control plane is disabled"))
		return
	}

	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, remoteKeyUploadRequestMaxBytes)
	if err := r.ParseMultipartForm(remoteKeyUploadRequestMaxBytes); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", fmt.Sprintf("request body exceeds %d bytes", remoteKeyUploadRequestMaxBytes)))
			return
		}
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request must be multipart/form-data"))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "file is required"))
		return
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, remoteKeyUploadMaxBytes+1))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "failed to read PEM file"))
		return
	}
	if len(content) > remoteKeyUploadMaxBytes {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", fmt.Sprintf("PEM file exceeds %d bytes", remoteKeyUploadMaxBytes)))
		return
	}

	filename := ""
	if header != nil {
		filename = strings.TrimSpace(header.Filename)
	}
	uploaded, saveErr := saveUploadedRemoteKey(filename, content)
	if saveErr != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", saveErr.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"key":       uploaded,
	})
}
