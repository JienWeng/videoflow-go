package routing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"videoflow-go/internal/costs"
	"videoflow-go/internal/providers"
)

// A key check is remembered per service, together with a short hash of the key it
// checked, so a newly saved key is never judged by the old key's answer.
type keyRecord struct {
	providers.KeyCheck
	Key string `json:"key"`
}

func keyCheckSetting(provider string) string { return "key_check_" + provider }

func keyHash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:8])
}

func (r Resolver) SaveKeyCheck(provider, key string, check providers.KeyCheck) error {
	raw, err := json.Marshal(keyRecord{check, keyHash(key)})
	if err != nil {
		return err
	}
	return r.DB.SetAppSetting(keyCheckSetting(provider), string(raw))
}

func (r Resolver) keyCheckFor(provider, key string) (providers.KeyCheck, bool) {
	raw, err := r.DB.GetAppSetting(keyCheckSetting(provider))
	var record keyRecord
	if err != nil || json.Unmarshal([]byte(raw), &record) != nil || record.Key != keyHash(key) {
		return providers.KeyCheck{}, false
	}
	return record.KeyCheck, true
}

// KeyCheck is the last answer about the key in use now; false when it was never checked.
func (r Resolver) KeyCheck(provider string) (providers.KeyCheck, bool) {
	key, _, _, err := r.Credentials(provider)
	if err != nil {
		return providers.KeyCheck{}, false
	}
	return r.keyCheckFor(provider, key)
}

// KeyCheckDue says whether the saved key should be asked about again: never checked,
// checked over a day ago, or the service could not be reached over an hour ago. An
// account with no balance is asked again only when the owner presses Check again.
func (r Resolver) KeyCheckDue(provider string, now time.Time) bool {
	check, ok := r.KeyCheck(provider)
	age := now.Sub(check.CheckedAt)
	return !ok || check.State != providers.KeyNoCredit && (age > 24*time.Hour || check.State == providers.KeyUnchecked && age > time.Hour)
}

// Connected reports whether a service has a saved key that it has not refused, on an
// account that has not said it is out of credit.
func (r Resolver) Connected(provider string) bool {
	key, _, _, err := r.Credentials(provider)
	if err != nil {
		return false
	}
	check, _ := r.keyCheckFor(provider, key)
	return check.State != providers.KeyRejected && check.State != providers.KeyNoCredit
}

// ForgetNoCredit drops a recorded "no balance left" answer, so the next real request
// decides. Used when the owner presses Check again on a service with no free check.
func (r Resolver) ForgetNoCredit(provider string) {
	if check, ok := r.KeyCheck(provider); ok && check.State == providers.KeyNoCredit {
		_ = r.DB.SetAppSetting(keyCheckSetting(provider), "null")
	}
}

// NoteServiceAnswer records at once what a video service said during real work: a
// refused key, or an account with no balance left. A normal answer (nil) clears an
// earlier "no balance left".
func (r Resolver) NoteServiceAnswer(provider string, err error) {
	host := costs.HostByName(provider)
	if host == nil {
		return
	}
	if err == nil {
		r.ForgetNoCredit(provider)
		return
	}
	var check providers.KeyCheck
	switch {
	case providers.NoCredit(err):
		check = providers.OutOfCredit(host.Label)
	case providers.InvalidKey(err):
		check = providers.RejectedKey(host.Label)
	default:
		return
	}
	if key, _, _, e := r.Credentials(provider); e == nil {
		_ = r.SaveKeyCheck(provider, key, check)
	}
}
