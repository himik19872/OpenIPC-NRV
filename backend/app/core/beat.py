"""
Celery Beat — расписание периодических задач.
"""
from celery.schedules import crontab

from app.core.celery_app import celery_app

celery_app.conf.beat_schedule = {
    "cleanup-old-recordings": {
        "task": "app.tasks.cleanup.cleanup_old_recordings",
        "schedule": crontab(hour=3, minute=0),  # каждый день в 3:00
        "kwargs": {"days": 30},
    },
    "health-check-cameras": {
        "task": "app.tasks.video.check_cameras_online",
        "schedule": crontab(minute="*/5"),  # каждые 5 минут
    },
}