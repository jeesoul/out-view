package com.outview.service;

import com.outview.entity.BannedDevice;
import com.outview.repository.BannedDeviceRepository;
import org.springframework.stereotype.Service;

import java.util.List;

@Service
public class BanService {

    private final BannedDeviceRepository repo;

    public BanService(BannedDeviceRepository repo) {
        this.repo = repo;
    }

    public boolean isBanned(String deviceId) {
        return repo.existsByDeviceId(deviceId);
    }

    public void ban(String deviceId, String bannedBy, String reason) {
        if (!repo.existsByDeviceId(deviceId)) {
            repo.save(new BannedDevice(deviceId, bannedBy, reason));
        }
    }

    public void unban(String deviceId) {
        if (repo.existsByDeviceId(deviceId)) {
            repo.deleteById(deviceId);
        }
    }

    public List<BannedDevice> listBanned() {
        return repo.findAll();
    }
}
