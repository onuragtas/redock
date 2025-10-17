package controllers

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
)

// UsageList returns the list of usage projects
func UsageList(c *fiber.Ctx) error {
	// İlk network ölçümü
	netStat1, _ := net.IOCounters(false)

	// CPU hesaplaması için 1 saniye bekle (bu süre zarfında network hızını da hesaplayacağız)
	cpuPercent, _ := cpu.Percent(time.Second, false)

	// İkinci network ölçümü (1 saniye sonra)
	netStat2, _ := net.IOCounters(false)

	memStat, _ := mem.VirtualMemory()
	diskStat, _ := disk.Usage("/")

	// Anlık upload/download hızını hesapla (bytes/second)
	uploadSpeed := float64(netStat2[0].BytesSent - netStat1[0].BytesSent)
	downloadSpeed := float64(netStat2[0].BytesRecv - netStat1[0].BytesRecv)

	// Hızları dinamik olarak formatla
	formatSpeed := func(bytesPerSecond float64) string {
		if bytesPerSecond >= 1024*1024*1024 { // GB/s
			return fmt.Sprintf("%.2f GB/s", bytesPerSecond/(1024*1024*1024))
		} else if bytesPerSecond >= 1024*1024 { // MB/s
			return fmt.Sprintf("%.2f MB/s", bytesPerSecond/(1024*1024))
		} else if bytesPerSecond >= 1024 { // KB/s
			return fmt.Sprintf("%.2f KB/s", bytesPerSecond/1024)
		} else { // B/s
			return fmt.Sprintf("%.0f B/s", bytesPerSecond)
		}
	}

	// Boyutları dinamik olarak formatla (bytes)
	formatSize := func(bytes float64) string {
		if bytes >= 1024*1024*1024*1024 { // TB
			return fmt.Sprintf("%.2f TB", bytes/(1024*1024*1024*1024))
		} else if bytes >= 1024*1024*1024 { // GB
			return fmt.Sprintf("%.2f GB", bytes/(1024*1024*1024))
		} else if bytes >= 1024*1024 { // MB
			return fmt.Sprintf("%.2f MB", bytes/(1024*1024))
		} else if bytes >= 1024 { // KB
			return fmt.Sprintf("%.2f KB", bytes/1024)
		} else { // B
			return fmt.Sprintf("%.0f B", bytes)
		}
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"cpu_percent":        fmt.Sprintf("%.2f%%", cpuPercent[0]),
			"memory_used_gb":     formatSize(float64(memStat.Used)),
			"memory_total_gb":    formatSize(float64(memStat.Total)),
			"memory_percent":     fmt.Sprintf("%.2f%%", memStat.UsedPercent),
			"disk_used_gb":       formatSize(float64(diskStat.Used)),
			"disk_total_gb":      formatSize(float64(diskStat.Total)),
			"disk_percent":       fmt.Sprintf("%.2f%%", diskStat.UsedPercent),
			"network_sent_total": formatSize(float64(netStat2[0].BytesSent)),
			"network_recv_total": formatSize(float64(netStat2[0].BytesRecv)),
			"upload_speed":       formatSpeed(uploadSpeed),
			"download_speed":     formatSpeed(downloadSpeed),
		},
	})
}
