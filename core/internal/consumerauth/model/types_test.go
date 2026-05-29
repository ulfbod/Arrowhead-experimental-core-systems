package model_test

import (
	"testing"

	"arrowhead/core/internal/consumerauth/model"
)

func TestBuildInstanceID(t *testing.T) {
	id := model.BuildInstanceID("TemperatureProvider", "SERVICE_DEF", "temperatureService")
	want := "PR|LOCAL|TemperatureProvider|SERVICE_DEF|temperatureService"
	if id != want {
		t.Errorf("instanceId = %q, want %q", id, want)
	}
}

func TestInstanceIDURLEncoding(t *testing.T) {
	id := model.BuildInstanceID("Provider|With|Pipes", "SERVICE_DEF", "svc")
	encoded := model.EncodeInstanceID(id)
	if encoded == id {
		t.Error("URL-encoded instanceId should differ from raw when pipes are present")
	}
}
