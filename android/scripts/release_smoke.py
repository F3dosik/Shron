#!/usr/bin/env python3
"""Smoke-тест release-сборки на эмуляторе или устройстве.

Инструментальные тесты идут на debug-сборке, где R8 не работает, поэтому ошибки
оптимизации (вырезанные или переименованные классы, которые Go-ядро вызывает по
именам через JNI) они не ловят. Этот скрипт ставит release-APK и проходит через
UI Automator сценарий, который задевает Go-ядро без сервера: запуск, сохранение
настроек, генератор, сетевой вызов с разбором ошибки.

    ./gradlew assembleRelease -Pshron.debugSignedRelease=true
    python3 scripts/release_smoke.py app/build/outputs/apk/release/app-release.apk

Данные приложения на устройстве очищаются. adb берётся из $ANDROID_HOME/platform-tools
или из PATH.
"""

import os
import re
import shutil
import subprocess
import sys
import time
import xml.etree.ElementTree as ET

PACKAGE = "io.github.f3dosik.shron"
ACTIVITY = f"{PACKAGE}/.MainActivity"


def adb_path():
    home = os.environ.get("ANDROID_HOME") or os.environ.get("ANDROID_SDK_ROOT")
    if home and os.path.exists(os.path.join(home, "platform-tools", "adb")):
        return os.path.join(home, "platform-tools", "adb")
    found = shutil.which("adb")
    if not found:
        sys.exit("adb не найден: задайте ANDROID_HOME или добавьте adb в PATH")
    return found


ADB = adb_path()


def adb(*args, check=True):
    result = subprocess.run([ADB, *args], capture_output=True, text=True)
    if check and result.returncode != 0:
        sys.exit(f"adb {' '.join(args)}: {result.stdout}{result.stderr}")
    return result.stdout


def nodes():
    # FLAG_SECURE не мешает дампу иерархии: скриншоты запрещены, доступность — нет.
    adb("shell", "uiautomator", "dump", "/sdcard/shron-ui.xml", check=False)
    xml = adb("exec-out", "cat", "/sdcard/shron-ui.xml", check=False)
    if not xml.lstrip().startswith("<?xml"):
        return []
    return list(ET.fromstring(xml).iter("node"))


def find(text, substring=False):
    for node in nodes():
        value = node.get("text") or node.get("content-desc") or ""
        if value == text or (substring and text in value):
            return node
    return None


def check_alive():
    crash = adb("logcat", "-d", "-b", "crash", check=False).strip()
    if crash:
        sys.exit(f"Приложение упало:\n{crash}")


def swipe(down):
    """Прокручивает экран на полстраницы: на маленьком экране элементы бывают за краем."""
    size = re.search(r"(\d+)x(\d+)", adb("shell", "wm", "size"))
    width, height = (int(size.group(1)), int(size.group(2))) if size else (1080, 1920)
    x = str(width // 2)
    start, end = (height * 3 // 4, height // 4) if down else (height // 4, height * 3 // 4)
    adb("shell", "input", "swipe", x, str(start), x, str(end), "300")


def visible_texts():
    return [n.get("text") for n in nodes() if n.get("text")]


def dismiss_system_dialog():
    """Закрывает системное окно «… isn't responding» (на эмуляторе в CI его показывает,
    например, Pixel Launcher): оно закрывает экран приложения и к нему не относится."""
    for node in nodes():
        value = node.get("text") or ""
        if value.endswith("isn't responding") or value.endswith("не отвечает"):
            button = find("Wait") or find("Подождать")
            if button is not None:
                print(f"  закрываем системное окно «{value}»")
                left, top, right, bottom = map(int, re.findall(r"\d+", button.get("bounds")))
                adb("shell", "input", "tap", str((left + right) // 2), str((top + bottom) // 2))
                time.sleep(1)
            return True
    return False


def wait(text, substring=False, timeout=30):
    """Ждёт элемент. Если его нет на экране, листает вниз до конца, затем вверх: экран
    может быть прокручен так, что элемент за нижним или за верхним краем."""
    deadline = time.time() + timeout
    down = True
    while time.time() < deadline:
        check_alive()
        if dismiss_system_dialog():
            continue
        node = find(text, substring)
        if node is not None:
            print(f"  есть «{text}»")
            return node
        before = visible_texts()
        swipe(down)
        time.sleep(1)
        if visible_texts() == before:
            down = not down  # дошли до края — листаем обратно
    visible = [n.get("text") for n in nodes() if n.get("text")]
    sys.exit(f"Не дождались «{text}» за {timeout} с. На экране: {visible}")


def tap(text, substring=False):
    node = wait(text, substring)
    left, top, right, bottom = map(int, re.findall(r"\d+", node.get("bounds")))
    adb("shell", "input", "tap", str((left + right) // 2), str((top + bottom) // 2))


def type_into(label, value):
    tap(label)
    time.sleep(0.5)
    adb("shell", "input", "text", value)
    # Закрыть клавиатуру, чтобы она не закрывала кнопки.
    adb("shell", "input", "keyevent", "KEYCODE_BACK")


def main():
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    apk = sys.argv[1]

    print("Установка и запуск")
    adb("uninstall", PACKAGE, check=False)
    adb("install", apk)
    adb("logcat", "-c")
    adb("shell", "am", "start", "-W", "-n", ACTIVITY)

    print("Настройки сервера: адрес сохраняется через Go-ядро")
    wait("Подключение и блокировка")
    type_into("Адрес сервера", "127.0.0.1:1")
    tap("Сохранить")
    wait("Вход")

    print("Регистрация: генератор мастер-пароля из Go-ядра")
    tap("Создать аккаунт")
    wait("Регистрация")
    tap("Сгенерировать мастер-пароль")
    wait("Надёжность", substring=True)

    print("Вход: сетевой вызов и разбор ошибки Go")
    tap("Уже есть аккаунт — войти")
    wait("Вход")
    type_into("Логин", "smoke")
    type_into("Мастер-пароль", "smoke-password")
    tap("Войти")
    wait("Сервер недоступен")

    check_alive()
    print("OK")


if __name__ == "__main__":
    main()
