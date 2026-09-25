#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
"""Convert an EQ0-style classic profile description into an offline bundle.

Usage: profile-bundle-from-classic.py PROFILE.json OUTPUT_DIR --name NAME
       [--assets-root DIR]

The input and any referenced fonts/SVGs stay outside the repository. The
resulting profile.json has the POST /api/quote-profiles shape, with asset_id
values holding bundle-relative paths until the operator applies the bundle.
"""

import argparse
import json
from pathlib import Path
import shutil
from xml.etree import ElementTree


SAFE_ATTRS = {
    "viewBox", "width", "height", "fill", "stroke", "stroke-width",
    "stroke-linecap", "stroke-linejoin", "d", "cx", "cy", "r", "x", "y",
    "x1", "x2", "y1", "y2", "rx", "ry", "points", "transform", "opacity",
    "fill-opacity", "stroke-opacity",
}


def safe_source(root: Path, relative: str) -> Path:
    candidate = (root / relative).resolve(strict=True)
    if not candidate.is_relative_to(root.resolve(strict=True)) or not candidate.is_file():
        raise ValueError(f"asset escapes source root: {relative}")
    return candidate


def clean_svg(source: Path, color: str | None = None) -> bytes:
    root = ElementTree.parse(source).getroot()
    for element in root.iter():
        element.tag = element.tag.split("}")[-1]
        original = dict(element.attrib)
        element.attrib.clear()
        for key, value in original.items():
            if key in SAFE_ATTRS:
                element.set(key, color if value == "currentColor" and color else value)
        for pair in original.get("style", "").split(";"):
            key, sep, value = pair.partition(":")
            if sep and key.strip() in SAFE_ATTRS:
                value = value.strip()
                element.set(key.strip(), color if value == "currentColor" and color else value)
    root.set("xmlns", "http://www.w3.org/2000/svg")
    return ElementTree.tostring(root, encoding="utf-8")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("profile", type=Path, help="EQ0-style profile.json")
    parser.add_argument("output", type=Path, help="new empty bundle directory")
    parser.add_argument("--name", required=True, help="Aeon profile name")
    parser.add_argument("--assets-root", type=Path, help="root of referenced files (default: profile directory)")
    args = parser.parse_args()
    source = json.loads(args.profile.read_text(encoding="utf-8"))
    root = args.assets_root or args.profile.parent
    out = args.output
    if out.exists() and any(out.iterdir()):
        parser.error("output directory must be empty")
    out.mkdir(parents=True, exist_ok=True)
    asset_dir = out / "assets"
    asset_dir.mkdir(exist_ok=True)

    fonts = []
    for index, font in enumerate(source["fonts"]):
        original = safe_source(root, font["file"])
        target = f"assets/font-{index}{original.suffix.lower()}"
        shutil.copyfile(original, out / target)
        fonts.append({key: font[key] for key in ("role", "family", "weight", "style")} | {"asset_id": target})

    colors = source["colors"]["document_css"]
    assets = {item["role"]: item["path"] for item in source.get("assets", {}).get("quote_marks", [])}
    # EQ0 describes SVG paths in different metadata shapes. If present,
    # sanitise and bundle them; the profile remains valid without marks.
    def svg_asset(role: str, target: str, color: str | None = None) -> str:
        relative = assets.get(role)
        if not relative:
            return ""
        (out / target).write_bytes(clean_svg(safe_source(root, relative), color))
        return target

    accent = svg_asset("cover and section three-dot mark", "assets/cover-dots.svg", colors["teal"])
    muted = svg_asset("cover and section three-dot mark", "assets/footer-dots.svg", colors["muted"])
    mark = svg_asset("footer recolored mark", "assets/footer-mark.svg")
    page = source["page"]
    typography = source["typography"]
    definition = {
        "schema": "inspr.document-profile.v1",
        "layout_variant": "classic-v1",
        "locale": f"{source['locale']['language']}-{source['locale']['region']}",
        "fonts": fonts,
        "colors": {key: colors[value] for key, value in {
            "ink": "ink", "muted": "muted", "soft": "soft", "accent": "teal",
            "rule": "rule", "paper": "paper",
        }.items()},
        "typography": {key: str(typography[value]["size_pt"]) for key, value in {
            "body_pt": "body", "title_pt": "title", "section_pt": "section_heading",
            "table_pt": "table_body", "footer_pt": "footer",
        }.items()},
        "page": {"width_mm": str(page["width_mm"]), "height_mm": str(page["height_mm"])} | {
            key + "_mm": str(value) for key, value in page["margins_mm"].items()
        },
        "cover": {
            "top_mm": str(source["cover"]["top_offset_mm"]),
            "title_gap_mm": str(source["cover"]["title_top_margin_mm"]),
            "columns_gap_mm": str(source["cover"]["columns_top_margin_mm"]),
            "columns_padding_mm": str(source["cover"]["columns_top_padding_mm"]),
            **({"brand_asset_id": accent} if accent else {}),
        },
        "sections": {"numbering": "upper-roman", "heading_case": "upper"},
        "positions_table": {"columns": [
            {"key": key, "width_mm": str(width)} for key, width in (
                ("position", 9), ("description", 71), ("quantity", 15),
                ("unit", 22), ("unit_price", 24), ("total", 27),
            )
        ], "separator": "rule", "repeat_header": True},
        "totals": {"vat": "note", "discount": "hidden", "net_label": "Nettosumme"},
        "payment_terms": {"position": "sections", "heading": "Zahlungsbedingungen"},
        "acceptance": {"signature_columns": 2, "gap_mm": "14", "lead_mm": "28"},
        "footer": {
            **({"asset_id": mark} if mark else {}),
            **({"dots_asset_id": muted} if muted else {}),
            "width_mm": str(source["footer"]["logo_width_mm"]),
            "offset_mm": str(source["footer"]["logo_offset_mm"]),
            "page_number_format": source["locale"]["page_label"].replace("{n}", "{page}").replace("{total}", "{total}"),
        },
        "labels": {
            "quote": "ANGEBOT", "recipient": "Auftraggeber", "terms": "Bedingungen",
            "positions": "Leistungsaufstellung", "number": "Angebotsnummer",
            "date": "Angebotsdatum", "customer": "Kundennummer", "valid": "Gültig bis",
            "contact": "Ansprechpartner", "project": "Projektreferenz", "net": "Nettosumme",
            "signature_customer": "Ort, Datum, Unterschrift Auftraggeber",
            "signature_sender": "Ort, Datum, Unterschrift Auftragnehmer",
        },
    }
    payload = {"name": args.name, "definition": definition}
    (out / "profile.json").write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"bundle: {out} ({len(fonts)} fonts)")


if __name__ == "__main__":
    main()
