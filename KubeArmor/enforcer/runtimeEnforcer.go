// SPDX-License-Identifier: Apache-2.0
// Copyright 2021 Authors of KubeArmor

package enforcer

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	kl "github.com/kubearmor/KubeArmor/KubeArmor/common"
	fd "github.com/kubearmor/KubeArmor/KubeArmor/feeder"
	tp "github.com/kubearmor/KubeArmor/KubeArmor/types"
)

// RuntimeEnforcer Structure
type RuntimeEnforcer struct {
	// logger
	Logger *fd.Feeder

	// LSM type
	EnforcerType string

	// LSM - AppArmor
	appArmorEnforcer *AppArmorEnforcer

	// LSM - SELinux
	seLinuxEnforcer *SELinuxEnforcer
}

// NewRuntimeEnforcer Function
func NewRuntimeEnforcer(node tp.Node, logger *fd.Feeder) *RuntimeEnforcer {
	re := &RuntimeEnforcer{}

	re.Logger = logger

	if !kl.IsK8sLocal() {
		// mount securityfs
		if err := kl.RunCommandAndWaitWithErr("mount", []string{"-t", "securityfs", "securityfs", "/sys/kernel/security"}); err != nil {
			if _, err := os.Stat(filepath.Clean("/sys/kernel/security")); err != nil {
				re.Logger.Errf("Failed to read /sys/kernel/security (%s)", err.Error())
				return nil
			}
		}
	}

	lsm := []byte{}
	lsmPath := "/sys/kernel/security/lsm"

	if _, err := os.Stat(filepath.Clean(lsmPath)); err == nil {
		lsm, err = ioutil.ReadFile(lsmPath)
		if err != nil {
			re.Logger.Errf("Failed to read /sys/kernel/security/lsm (%s)", err.Error())
			return nil
		}
	}

	lsms := string(lsm)
	re.Logger.Printf("Supported LSMs: %s", lsms)

	if strings.Contains(lsms, "apparmor") {
		re.appArmorEnforcer = NewAppArmorEnforcer(node, logger)
		if re.appArmorEnforcer != nil {
			re.Logger.Print("Initialized AppArmor Enforcer")
			re.EnforcerType = "AppArmor"
			logger.UpdateEnforcer(re.EnforcerType)
			return re
		}
	} else if strings.Contains(lsms, "selinux") {
		if !kl.IsInK8sCluster() {
			re.seLinuxEnforcer = NewSELinuxEnforcer(node, logger)
			if re.seLinuxEnforcer != nil {
				re.Logger.Print("Initialized SELinux Enforcer")
				re.EnforcerType = "SELinux"
				logger.UpdateEnforcer(re.EnforcerType)
				return re
			}
		}
	}

	return nil
}

// UpdateAppArmorProfiles Function
func (re *RuntimeEnforcer) UpdateAppArmorProfiles(podName, action string, profiles map[string]string) {
	// skip if runtime enforcer is not active
	if re == nil {
		return
	}

	if re.EnforcerType == "AppArmor" {
		for _, profile := range profiles {
			if profile == "unconfined" {
				continue
			}

			if action == "ADDED" {
				re.appArmorEnforcer.RegisterAppArmorProfile(podName, profile)
			} else if action == "DELETED" {
				re.appArmorEnforcer.UnregisterAppArmorProfile(podName, profile)
			}
		}
	}
}

// UpdateSecurityPolicies Function
func (re *RuntimeEnforcer) UpdateSecurityPolicies(endPoint tp.EndPoint) {
	// skip if runtime enforcer is not active
	if re == nil {
		return
	}

	if re.EnforcerType == "AppArmor" {
		re.appArmorEnforcer.UpdateSecurityPolicies(endPoint)
	} else if re.EnforcerType == "SELinux" {
		// do nothing
	}
}

// UpdateHostSecurityPolicies Function
func (re *RuntimeEnforcer) UpdateHostSecurityPolicies(secPolicies []tp.HostSecurityPolicy) {
	// skip if runtime enforcer is not active
	if re == nil {
		return
	}

	if re.EnforcerType == "AppArmor" {
		re.appArmorEnforcer.UpdateHostSecurityPolicies(secPolicies)
	} else if re.EnforcerType == "SELinux" {
		re.seLinuxEnforcer.UpdateHostSecurityPolicies(secPolicies)
	}
}

// DestroyRuntimeEnforcer Function
func (re *RuntimeEnforcer) DestroyRuntimeEnforcer() error {
	// skip if runtime enforcer is not active
	if re == nil {
		return nil
	}

	errorLSM := false

	if re.EnforcerType == "AppArmor" {
		if re.appArmorEnforcer != nil {
			if err := re.appArmorEnforcer.DestroyAppArmorEnforcer(); err != nil {
				re.Logger.Err(err.Error())
				errorLSM = true
			} else {
				re.Logger.Print("Destroyed AppArmor Enforcer")
			}
		}
	} else if re.EnforcerType == "SELinux" {
		if re.seLinuxEnforcer != nil {
			if err := re.seLinuxEnforcer.DestroySELinuxEnforcer(); err != nil {
				re.Logger.Err(err.Error())
				errorLSM = true
			} else {
				re.Logger.Print("Destroyed SELinux Enforcer")
			}
		}
	}

	if errorLSM {
		return fmt.Errorf("failed to destroy RuntimeEnforcer (%s)", re.EnforcerType)
	}

	return nil
}
