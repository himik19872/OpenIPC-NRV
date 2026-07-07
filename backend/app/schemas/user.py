"""
NRV Backend — Схемы пользователя.
"""
import uuid
from datetime import datetime
from typing import Optional

from pydantic import BaseModel, EmailStr, Field


class UserCreate(BaseModel):
    """Запрос на создание пользователя."""

    username: str = Field(..., min_length=3, max_length=64)
    email: EmailStr
    password: str = Field(..., min_length=6, max_length=128)
    full_name: str = Field(default="", max_length=128)


class UserUpdate(BaseModel):
    """Запрос на обновление пользователя."""

    email: Optional[EmailStr] = None
    full_name: Optional[str] = Field(default=None, max_length=128)
    role: Optional[str] = Field(default=None, pattern=r"^(admin|operator|viewer)$")
    is_active: Optional[bool] = None


class UserOut(BaseModel):
    """Ответ с данными пользователя (без пароля)."""

    id: uuid.UUID
    username: str
    email: str
    full_name: str
    role: str
    is_active: bool
    is_superuser: bool
    created_at: datetime
    last_login: Optional[datetime] = None

    model_config = {"from_attributes": True}


class UserLogin(BaseModel):
    """Запрос на вход."""

    username: str
    password: str


class TokenResponse(BaseModel):
    """Ответ с токенами."""

    access_token: str
    refresh_token: str
    token_type: str = "bearer"
    user: UserOut


class RefreshRequest(BaseModel):
    """Запрос на обновление access-токена."""

    refresh_token: str


class ChangePasswordRequest(BaseModel):
    """Запрос на смену пароля."""

    old_password: str
    new_password: str = Field(..., min_length=6, max_length=128)