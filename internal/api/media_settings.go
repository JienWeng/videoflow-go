package api

import (
	"context"
	"fmt"
	"videoflow-go/internal/providers"
	"videoflow-go/internal/routing"
)

func (s *Server) mediaCatalogClient(provider, model string) (providers.MediaClient, error) {
	switch provider {
	case "fal":
		return providers.NewFalClientAt("", providers.FalURL, model), nil
	case "byteplus":
		return providers.NewBytePlusClientAt("", providers.BytePlusURL), nil
	default:
		return s.routes().Client(provider)
	}
}
func (s *Server) prepareMediaSettings(ctx context.Context, body map[string]any) error {
	for _, kind := range []string{"image", "video"} {
		provider, current := s.routes().MediaSelection(kind)
		previous := provider
		providerKey := "default_" + kind + "_provider"
		if raw, ok := body[providerKey]; ok {
			var valid bool
			provider, valid = raw.(string)
			if !valid || (provider != "openrouter" && provider != "fal" && provider != "byteplus") || (kind == "image" && provider == "byteplus") {
				return fmt.Errorf("unsupported %s provider", kind)
			}
		}
		keys := []string{kind + "_model"}
		if kind == "image" {
			keys = append(keys, "ref_image_model")
		}
		if previous != provider {
			body[routing.MediaPreferenceKey(previous, kind)] = current
			if kind == "image" {
				body[routing.MediaPreferenceKey(previous, "reference")] = s.routes().MediaModel("reference")
			}
		}
		for _, key := range keys {
			modelKind := kind
			if key == "ref_image_model" {
				modelKind = "reference"
			}
			if previous != provider {
				if _, explicit := body[key]; !explicit {
					body[key] = s.routes().Setting(routing.MediaPreferenceKey(provider, modelKind), s.routes().DefaultMediaModel(provider, modelKind))
				}
			}
			if raw, ok := body[key]; ok {
				model, valid := raw.(string)
				if !valid || model == "" {
					return fmt.Errorf("model ID is required")
				}
				client, err := s.mediaCatalogClient(provider, model)
				if err != nil {
					return err
				}
				if _, err = client.Model(ctx, model, kind); err != nil {
					return err
				}
				body[routing.MediaPreferenceKey(provider, modelKind)] = model
			}
		}
	}
	return nil
}
