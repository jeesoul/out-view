package com.outview.repository;

import com.outview.entity.PortMapping;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.stereotype.Repository;

/**
 * 端口映射持久化仓库
 */
@Repository
public interface PortMappingRepository extends JpaRepository<PortMapping, String> {
}
