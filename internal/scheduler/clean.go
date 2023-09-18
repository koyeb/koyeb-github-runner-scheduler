package scheduler

import (
	"fmt"
	"sync"
	"time"

	"github.com/koyeb/koyeb-github-runner-scheduler/internal/koyeb_api"
)

type Cleaner struct {
	koyebAPIClient koyeb_api.APIClient
	services       map[string]time.Time
	lock           sync.Mutex
	ttl            time.Duration
}

func SetupCleaner(koyebAPIClient koyeb_api.APIClient, services []string, ttl time.Duration) *Cleaner {
	cleaner := &Cleaner{
		koyebAPIClient: koyebAPIClient,
		services:       make(map[string]time.Time),
		ttl:            ttl,
	}
	for _, service := range services {
		cleaner.Update(service)
	}
	return cleaner
}

func (cleaner *Cleaner) watch(service string) {
	for {
		time.Sleep(1 * time.Second)

		cleaner.lock.Lock()
		lastUsed := cleaner.services[service]
		cleaner.lock.Unlock()

		if time.Since(lastUsed) >= cleaner.ttl {
			fmt.Printf("TTL reached for service %s, removing it\n", service)
			removed, err := cleaner.koyebAPIClient.DeleteService(service)
			if err != nil {
				fmt.Printf("Oops, failed to delete service %s - keep trying: %s\n", service, err)
				continue
			}
			if !removed {
				fmt.Printf("Service %s was already deleted, skip\n", service)
			}

			cleaner.lock.Lock()
			delete(cleaner.services, service)
			cleaner.lock.Unlock()
			return
		}
	}
}

func (cleaner *Cleaner) Update(serviceId string) {
	cleaner.lock.Lock()
	defer cleaner.lock.Unlock()

	// If the service is not already being watched, start a new goroutine to clean it.
	if _, exists := cleaner.services[serviceId]; !exists {
		go cleaner.watch(serviceId)
	}

	cleaner.services[serviceId] = time.Now()
}
