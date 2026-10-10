package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DeviceTokenTTL is the lifetime of a device-login token (APP_DEVICE_TOKEN_TTL_DAYS).
func DeviceTokenTTL() time.Duration { return deviceTokenTTL() }

// DeviceCodeTTL is how long a device login request stays approvable.
func DeviceCodeTTL() time.Duration { return deviceAuthTTL }

func (rt *Router) libraryDeviceStart(w http.ResponseWriter, r *http.Request, req deviceStartRequest) {
	res, err := rt.authLib.device.StartDevice(r.Context(), authengine.DeviceStartInput{
		ClientName: req.ClientName, IP: clientIP(r), UserAgent: r.UserAgent(),
	})
	if err != nil {
		rt.internalError(w, "api: device auth start: library start failed", err)
		return
	}
	base := deviceVerificationBase(r)
	rt.notifyDeviceLoginWaiting()
	writeJSON(w, http.StatusCreated, deviceStartResponse{
		DeviceCode:              res.DeviceCode,
		UserCode:                res.UserCode,
		VerificationURI:         base + deviceCLIPath,
		VerificationURIComplete: base + deviceCLIPath + "?user_code=" + res.UserCode,
		ExpiresIn:               int(res.ExpiresIn.Seconds()),
		Interval:                int(res.Interval.Seconds()),
	})
}

func (rt *Router) libraryDeviceToken(w http.ResponseWriter, r *http.Request, deviceCode string) {
	legacyID, err := randomTokenID()
	if err != nil {
		rt.internalError(w, "api: device auth token: generate token id failed", err)
		return
	}
	raw, rec, err := rt.authLib.device.RedeemDevice(r.Context(), deviceCode)
	switch {
	case errors.Is(err, authengine.ErrDevicePending):
		writeError(w, http.StatusBadRequest, "authorization_pending")
		return
	case errors.Is(err, authengine.ErrDeviceDenied):
		writeError(w, http.StatusBadRequest, "access_denied")
		return
	case errors.Is(err, authengine.ErrDeviceExpired):
		if errors.Is(err, authengine.ErrDeviceLapsed) {
			rt.recordAudit(r.Context(), r, AbilityRead, auditActorDevice, "", "device login expired", http.StatusBadRequest)
		}
		writeError(w, http.StatusBadRequest, "expired_token")
		return
	case err != nil:
		rt.internalError(w, "api: device auth token: library redeem failed", err)
		return
	}
	legacy := store.APIToken{
		ID: legacyID, Name: rec.Name, TokenHash: rec.HashHex, Abilities: rec.Abilities, CreatedAt: rec.CreatedAt,
		ExpiresAt: rec.ExpiresAt, OwnerUserID: rec.OwnerLegacyID,
	}
	if err := rt.tokens.SaveAPIToken(r.Context(), legacy); err != nil {
		_ = rt.authLib.device.RevokeToken(r.Context(), rec.EngineID)
		rt.internalError(w, "api: device auth token: mirror save failed", err)
		return
	}
	if err := rt.authLib.device.LinkToken(r.Context(), legacyID, rec.EngineID); err != nil {
		rt.internalError(w, "api: device auth token: link failed", err)
		return
	}
	writeJSON(w, http.StatusOK, createTokenResponse{tokenResource: toTokenResource(legacy), Token: raw})
}

func (rt *Router) libraryListDeviceRequests(w http.ResponseWriter, r *http.Request) {
	pending, err := rt.authLib.device.PendingDevices(r.Context())
	if err != nil {
		rt.internalError(w, "api: list device auth requests failed", err)
		return
	}
	viewer := clientIP(r)
	out := make([]deviceAuthRequestResource, 0, len(pending))
	for _, p := range pending {
		out = append(out, deviceAuthRequestResource{
			UserCode: p.UserCode, ClientName: p.ClientName, CreatedAt: p.CreatedAt, ExpiresAt: p.ExpiresAt,
			RequesterIP: p.RequesterIP, UserAgent: p.UserAgent, IPMismatch: p.RequesterIP != "" && p.RequesterIP != viewer,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) libraryDecideDevice(w http.ResponseWriter, r *http.Request, userID, userCode string, approve bool) {
	var abilities []string
	if approve {
		user, err := rt.auth.GetUserByID(r.Context(), userID)
		if err != nil {
			rt.internalError(w, "api: decide device auth request: load approver failed", err)
			return
		}
		abilities = capDeviceAbilities(user.Abilities)
	}
	err := rt.authLib.device.DecideDevice(r.Context(), authengine.DeviceDecision{
		ApproverLegacyID: userID, IP: clientIP(r), UserCode: userCode, Approve: approve, Abilities: abilities,
	})
	switch {
	case errors.Is(err, authengine.ErrDeviceNotFound):
		writeError(w, http.StatusNotFound, "no pending device login with this code")
		return
	case errors.Is(err, authengine.ErrDeviceThrottled):
		writeError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
		return
	case errors.Is(err, authengine.ErrDeviceAbilities):
		writeError(w, http.StatusForbidden, "your account lacks the abilities this login needs")
		return
	case err != nil:
		rt.internalError(w, "api: decide device auth request failed", err)
		return
	}
	rt.recordAudit(r.Context(), r, AbilityWrite, auditActorSession, userID, "", http.StatusNoContent)
	w.WriteHeader(http.StatusNoContent)
}
