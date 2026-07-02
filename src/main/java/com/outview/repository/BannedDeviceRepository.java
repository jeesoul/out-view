package com.outview.repository;

import com.outview.entity.BannedDevice;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.stereotype.Repository;

@Repository
public interface BannedDeviceRepository extends JpaRepository<BannedDevice, String> {
    boolean existsByDeviceId(String deviceId);
}
