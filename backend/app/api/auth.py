"""
NRV Backend — Auth API: регистрация, вход, обновление токенов.
"""
from datetime import datetime, timezone
from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.deps import get_current_user
from app.core.database import get_db
from app.core.security import (
    create_access_token,
    create_refresh_token,
    decode_token,
    hash_password,
    verify_password,
)
from app.models.user import User
from app.schemas.user import (
    ChangePasswordRequest,
    RefreshRequest,
    TokenResponse,
    UserCreate,
    UserLogin,
    UserOut,
    UserUpdate,
)

router = APIRouter(prefix="/auth", tags=["auth"])


@router.post("/register", response_model=TokenResponse, status_code=status.HTTP_201_CREATED)
async def register(payload: UserCreate, db: Annotated[AsyncSession, Depends(get_db)]):
    """Регистрация нового пользователя."""
    # Проверка уникальности username
    existing = await db.execute(select(User).where(User.username == payload.username))
    if existing.scalar_one_or_none():
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="Username already taken",
        )

    # Проверка уникальности email
    existing = await db.execute(select(User).where(User.email == payload.email))
    if existing.scalar_one_or_none():
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="Email already registered",
        )

    user = User(
        username=payload.username,
        email=payload.email,
        hashed_password=hash_password(payload.password),
        full_name=payload.full_name,
        role="operator",
    )
    db.add(user)
    await db.flush()
    await db.refresh(user)

    # Создаём токены
    access_token = create_access_token(subject=str(user.id), extra_claims={"role": user.role})
    refresh_token = create_refresh_token(subject=str(user.id), extra_claims={"role": user.role})

    user_out = UserOut.model_validate(user)
    return TokenResponse(
        access_token=access_token,
        refresh_token=refresh_token,
        user=user_out,
    )


@router.post("/login", response_model=TokenResponse)
async def login(payload: UserLogin, db: Annotated[AsyncSession, Depends(get_db)]):
    """Вход в систему."""
    result = await db.execute(select(User).where(User.username == payload.username))
    user = result.scalar_one_or_none()

    if not user or not verify_password(payload.password, user.hashed_password):
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Invalid username or password",
        )

    if not user.is_active:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="User account is deactivated",
        )

    # Обновляем last_login
    user.last_login = datetime.now(timezone.utc)
    await db.flush()

    access_token = create_access_token(subject=str(user.id), extra_claims={"role": user.role})
    refresh_token = create_refresh_token(subject=str(user.id), extra_claims={"role": user.role})

    user_out = UserOut.model_validate(user)
    return TokenResponse(
        access_token=access_token,
        refresh_token=refresh_token,
        user=user_out,
    )


@router.post("/refresh", response_model=TokenResponse)
async def refresh_token(
    payload: RefreshRequest,
    db: Annotated[AsyncSession, Depends(get_db)],
):
    """Обновление access-токена по refresh-токену."""
    try:
        decoded = decode_token(payload.refresh_token)
    except Exception:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Invalid refresh token",
        )

    if decoded.get("type") != "refresh":
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Invalid token type",
        )

    user_id = decoded.get("sub")
    if not user_id:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED)

    result = await db.execute(select(User).where(User.id == UUID(user_id)))
    user = result.scalar_one_or_none()

    if not user or not user.is_active:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="User not found or inactive",
        )

    access_token = create_access_token(subject=str(user.id), extra_claims={"role": user.role})
    new_refresh_token = create_refresh_token(subject=str(user.id), extra_claims={"role": user.role})

    user_out = UserOut.model_validate(user)
    return TokenResponse(
        access_token=access_token,
        refresh_token=new_refresh_token,
        user=user_out,
    )


@router.get("/me", response_model=UserOut)
async def get_me(current_user: Annotated[User, Depends(get_current_user)]):
    """Возвращает профиль текущего пользователя."""
    return UserOut.model_validate(current_user)


@router.put("/me", response_model=UserOut)
async def update_me(
    payload: UserUpdate,
    current_user: Annotated[User, Depends(get_current_user)],
    db: Annotated[AsyncSession, Depends(get_db)],
):
    """Обновление профиля текущего пользователя."""
    if payload.email is not None:
        current_user.email = payload.email
    if payload.full_name is not None:
        current_user.full_name = payload.full_name

    await db.flush()
    await db.refresh(current_user)
    return UserOut.model_validate(current_user)


@router.post("/change-password", status_code=status.HTTP_200_OK)
async def change_password(
    payload: ChangePasswordRequest,
    current_user: Annotated[User, Depends(get_current_user)],
    db: Annotated[AsyncSession, Depends(get_db)],
):
    """Смена пароля."""
    if not verify_password(payload.old_password, current_user.hashed_password):
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Old password is incorrect",
        )

    current_user.hashed_password = hash_password(payload.new_password)
    await db.flush()

    return {"message": "Password changed successfully"}


@router.post("/logout", status_code=status.HTTP_200_OK)
async def logout(current_user: Annotated[User, Depends(get_current_user)]):
    """Выход (на клиенте удалить токены; здесь — заглушка для будущего blacklist)."""
    return {"message": "Logged out successfully"}