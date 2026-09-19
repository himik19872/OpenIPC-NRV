"""Юнит-тесты геометрии детекции: зона, линия, пересечения, центры рамок."""
from detection_config import (
    point_in_polygon, line_side, LineCrossingTracker, bbox_center, DetectionConfig,
)

failures = []


def check(name, cond, extra=""):
    if cond:
        print(f"  OK   {name} {extra}")
    else:
        print(f"  FAIL {name} {extra}")
        failures.append(name)


# --- Зона (полигон) ---
zone = [{"x": 0.2, "y": 0.2}, {"x": 0.8, "y": 0.2}, {"x": 0.8, "y": 0.8}, {"x": 0.2, "y": 0.8}]
check("зона: центр внутри", point_in_polygon({"x": 0.5, "y": 0.5}, zone) is True)
check("зона: слева снаружи", point_in_polygon({"x": 0.1, "y": 0.5}, zone) is False)
check("зона: угол снаружи", point_in_polygon({"x": 0.9, "y": 0.9}, zone) is False)
check("зона: пустая = весь кадр", point_in_polygon({"x": 0.99, "y": 0.99}, []) is True)

# --- Сторона линии ---
# Соглашение по знаку: для линии слева-направо точка ВЫШЕ неё даёт
# отрицательное произведение, ниже — положительное. Важен не сам знак,
# а его смена между кадрами — именно она означает пересечение.
line = [{"x": 0.0, "y": 0.5}, {"x": 1.0, "y": 0.5}]
check("линия: сверху < 0", line_side({"x": 0.5, "y": 0.2}, line) < 0,
      f"→ {line_side({'x': 0.5, 'y': 0.2}, line):.2f}")
check("линия: снизу > 0", line_side({"x": 0.5, "y": 0.8}, line) > 0,
      f"→ {line_side({'x': 0.5, 'y': 0.8}, line):.2f}")
check("линия: на линии = 0", abs(line_side({"x": 0.5, "y": 0.5}, line)) < 1e-9)

# --- Пересечение линии ---
t = LineCrossingTracker()
r1 = t.check("cam1", 1, {"x": 0.5, "y": 0.2}, line, "both")
check("пересечение: первый кадр не считается", r1 is None)
r2 = t.check("cam1", 1, {"x": 0.5, "y": 0.8}, line, "both")
check("пересечение: сверху вниз", r2 is not None, f"→ {r2}")
r3 = t.check("cam1", 1, {"x": 0.5, "y": 0.2}, line, "both")
check("пересечение: обратно", r3 is not None and r3 != r2, f"→ {r3}")

# Один и тот же трек в одну сторону дважды не должен давать два события
t_same = LineCrossingTracker()
t_same.check("c", 1, {"x": 0.5, "y": 0.2}, line, "both")
t_same.check("c", 1, {"x": 0.5, "y": 0.8}, line, "both")
dup = t_same.check("c", 1, {"x": 0.5, "y": 0.7}, line, "both")
check("пересечение: без повтора в той же стороне", dup is None)

# Треки разных камер не смешиваются
t_sep = LineCrossingTracker()
t_sep.check("camA", 7, {"x": 0.5, "y": 0.2}, line, "both")
crs = t_sep.check("camB", 7, {"x": 0.5, "y": 0.8}, line, "both")
check("пересечение: треки камер независимы", crs is None)

# Фильтр направления
t_dir = LineCrossingTracker()
t_dir.check("c", 5, {"x": 0.5, "y": 0.2}, line, "forward")
res_fwd = t_dir.check("c", 5, {"x": 0.5, "y": 0.8}, line, "forward")
check("направление forward пропускает прямое", res_fwd is not None, f"→ {res_fwd}")

t_bwd = LineCrossingTracker()
t_bwd.check("c", 5, {"x": 0.5, "y": 0.2}, line, "backward")
res_bwd = t_bwd.check("c", 5, {"x": 0.5, "y": 0.8}, line, "backward")
check("направление backward блокирует прямое", res_bwd is None)

# --- Центр рамки в нормализованных координатах ---
c = bbox_center({"x": 100, "y": 50, "w": 200, "h": 100}, 1000, 500)
check("bbox_center: 1000x500 → 0.2,0.2",
      abs(c["x"] - 0.2) < 1e-6 and abs(c["y"] - 0.2) < 1e-6, f"→ {c}")
c2 = bbox_center({"x": 0, "y": 0, "w": 0, "h": 0}, 0, 0)
check("bbox_center: нулевой кадр не делит на ноль",
      c2["x"] == 0 and c2["y"] == 0, f"→ {c2}")

print()
if failures:
    print(f"ПРОВАЛЕНО: {len(failures)} — {failures}")
    raise SystemExit(1)
print("Все тесты пройдены")
