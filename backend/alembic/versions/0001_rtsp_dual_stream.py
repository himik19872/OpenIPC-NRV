"""Добавление rtsp_main_url, rtsp_sub_url, rtsp_proxy_main_url, rtsp_proxy_sub_url в cameras.

Миграция переименовывает rtsp_url → rtsp_main_url и добавляет поля для:
- дополнительного RTSP-потока (rtsp_sub_url)
- прокси-RTSP URL через go2rtc (rtsp_proxy_main_url, rtsp_proxy_sub_url)

Revision ID: 0001_rtsp_dual_stream
Create Date: 2026-07-08
"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = '0001_rtsp_dual_stream'
down_revision: Union[str, None] = None
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    # 1. Переименовываем rtsp_url → rtsp_main_url
    op.alter_column('cameras', 'rtsp_url', new_column_name='rtsp_main_url')

    # 2. Добавляем rtsp_sub_url (дополнительный поток)
    op.add_column('cameras', sa.Column(
        'rtsp_sub_url',
        sa.String(512),
        nullable=True,
        default=None,
    ))

    # 3. Прокси-RTSP URL'ы (заполняются автоматически при регистрации в go2rtc)
    op.add_column('cameras', sa.Column(
        'rtsp_proxy_main_url',
        sa.String(512),
        nullable=True,
        default=None,
    ))
    op.add_column('cameras', sa.Column(
        'rtsp_proxy_sub_url',
        sa.String(512),
        nullable=True,
        default=None,
    ))


def downgrade() -> None:
    # Удаляем прокси-URL'ы
    op.drop_column('cameras', 'rtsp_proxy_sub_url')
    op.drop_column('cameras', 'rtsp_proxy_main_url')

    # Удаляем rtsp_sub_url
    op.drop_column('cameras', 'rtsp_sub_url')

    # Переименовываем обратно
    op.alter_column('cameras', 'rtsp_main_url', new_column_name='rtsp_url')
