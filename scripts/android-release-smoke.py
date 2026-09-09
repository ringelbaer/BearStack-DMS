"""Exercise the actual R8 APK through Android's external UI automation.

Requires a dedicated emulator; only temporary fixture accounts are used. The
runner lives outside the app process, so the release APK needs no test keep rules
or testing libraries. Python and adb are the only host dependencies.
"""
from pathlib import Path
import re
import shlex
import subprocess
import sys
import time
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[1]
PACKAGE = "de.bearstack.people"
REPORT = ROOT / "apps/android/app/build/reports/release-smoke.xml"


def adb(*args):
    return subprocess.check_output(["adb", *map(str, args)], text=True, stderr=subprocess.STDOUT, timeout=45)


def shell(*args):
    return adb("shell", " ".join(shlex.quote(str(arg)) for arg in args))


def hierarchy():
    shell("uiautomator", "dump", "/data/local/tmp/bearstack-release-ui.xml")
    return ET.fromstring(adb("exec-out", "cat", "/data/local/tmp/bearstack-release-ui.xml"))


def nodes(tree, *labels):
    return [node for node in tree.iter("node")
            if node.get("text") in labels or node.get("content-desc") in labels]


def wait_for(find, description):
    deadline = time.monotonic() + 35
    last = None
    while time.monotonic() < deadline:
        last = hierarchy()
        found = find(last)
        if found:
            return found
        time.sleep(0.3)
    visible = [(node.get("text"), node.get("content-desc")) for node in last.iter("node")
               if node.get("text") or node.get("content-desc")]
    raise AssertionError(f"Missing {description}; visible: {visible}")


def tap(node):
    x1, y1, x2, y2 = map(int, re.findall(r"\d+", node.get("bounds", "")))
    shell("input", "tap", (x1+x2)//2, (y1+y2)//2)


def click(*labels):
    tap(wait_for(lambda tree: nodes(tree, *labels), " / ".join(labels))[0])


def launch():
    shell("am", "start", "-W", "-n", f"{PACKAGE}/.MainActivity")


def login(address, account):
    for index, value in enumerate([address, account, "secret"]):
        fields = wait_for(lambda tree: [node for node in tree.iter("node")
                          if node.get("class") == "android.widget.EditText"], "login fields")
        tap(fields[index])
        shell("input", "text", value)
        shell("input", "keyevent", "KEYCODE_BACK")
    click("Verbinden", "Connect")
    click("Abgeglichen und vertrauen", "Verified; trust certificate")
    wait_for(lambda tree: nodes(tree, "one.jpg"), "native gallery photo")


def run(address):
    if shell("getprop", "ro.kernel.qemu").strip() != "1":
        raise RuntimeError("Release smoke requires a dedicated emulator; refusing to reset a physical device")
    # This deliberately resets only our test app on the dedicated emulator.
    adb("install", "-r", "-t", ROOT / "apps/android/app/build/outputs/apk/release/app-release.apk")
    shell("pm", "clear", PACKAGE)
    try:
        launch()
        login(address, "reader")
        print("PASS release: HTTPS reader login and native gallery", flush=True)
        click("Weitere Optionen", "More options")
        tree = hierarchy()
        if nodes(tree, "Personen verwalten", "Manage people"):
            raise AssertionError("Reader was offered person editing")
        shell("input", "keyevent", "KEYCODE_BACK")
        click("one.jpg")
        click("Informationen", "Information")
        wait_for(lambda tree: [node for node in tree.iter("node") if "52.5" in node.get("text", "")],
                 "GPS information from the fixture")
        print("PASS release: viewer and server photo information", flush=True)
        shell("am", "force-stop", PACKAGE)
        launch()
        wait_for(lambda tree: nodes(tree, "one.jpg"), "restored gallery session")
        print("PASS release: encrypted profile and session restoration", flush=True)
        click("Weitere Optionen", "More options")
        click("Verbindung wechseln", "Switch connection")
        login(address, "manager")
        click("Weitere Optionen", "More options")
        wait_for(lambda tree: nodes(tree, "Personen verwalten", "Manage people"), "manager permissions")
        click("Verbindung wechseln", "Switch connection")
        wait_for(lambda tree: nodes(tree, "Verbinden", "Connect"), "signed-out screen")
        print("PASS release: account switch, permissions and sign-out", flush=True)
    finally:
        shell("am", "force-stop", PACKAGE)
        shell("pm", "clear", PACKAGE)
        shell("rm", "-f", "/data/local/tmp/bearstack-release-ui.xml")


if __name__ == "__main__":
    suite = ET.Element("testsuite", name="AndroidReleaseSmoke", tests="1", failures="0", errors="0")
    case = ET.SubElement(suite, "testcase", name="login_gallery_info_restore_switch", classname="AndroidReleaseSmoke")
    started = time.monotonic()
    try:
        run(sys.argv[1])
    except Exception as error:
        suite.set("failures", "1")
        ET.SubElement(case, "failure", message=str(error)).text = str(error)
        raise
    finally:
        suite.set("time", str(time.monotonic()-started))
        REPORT.parent.mkdir(parents=True, exist_ok=True)
        ET.ElementTree(suite).write(REPORT, encoding="utf-8", xml_declaration=True)
