"""Shared resource presets for deployments."""

resources_central_small = {
    "requests": {"cpu": "500m", "memory": "700Mi"},
    "limits": {"cpu": "2000m", "memory": "3500Mi"},
}

resources_central_db_small = {
    "requests": {"cpu": "500m", "memory": "700Mi"},
    "limits": {"cpu": "2000m", "memory": "3500Mi"},
}

resources_scanner_analyzer_small = {
    "requests": {"cpu": "500m", "memory": "500Mi"},
    "limits": {"cpu": "2000m", "memory": "2500Mi"},
}

resources_scanner_db_small = {
    "requests": {"cpu": "400m", "memory": "512Mi"},
    "limits": {"cpu": "2000m", "memory": "4Gi"},
}

resources_scanner_v4_indexer_small = {
    "requests": {"cpu": "400m", "memory": "512Mi"},
    "limits": {"cpu": "2000m", "memory": "4Gi"},
}
resources_scanner_v4_matcher_small = {
    "requests": {"cpu": "400m", "memory": "512Mi"},
    "limits": {"cpu": "1000m", "memory": "2Gi"},
}

resources_scanner_v4_db_small = {
    "requests": {"cpu": "400m", "memory": "512Mi"},
    "limits": {"cpu": "1000m", "memory": "2Gi"},
}

resources_sensor_small = {
    "requests": {"cpu": "500m", "memory": "500Mi"},
    "limits": {"cpu": "1000m", "memory": "2Gi"},
}
