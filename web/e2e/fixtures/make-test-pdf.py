import sys
pages = int(sys.argv[2]); out = sys.argv[1]
objs = []
def add(s): objs.append(s); return len(objs)
font = add("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
pages_id = len(objs) + 1 + 2*pages  # placeholder computed later
kids = []
content_ids = []
page_objs = []
for i in range(1, pages+1):
    lines = [f"BT /F1 28 Tf 72 720 Td (Chapter {i}) Tj ET"]
    y = 680
    for j in range(30):
        lines.append(f"BT /F1 12 Tf 72 {y} Td (Page {i} line {j+1}: the quick brown fox jumps over the lazy dog.) Tj ET")
        y -= 20
    stream = "\n".join(lines)
    c = add(f"<< /Length {len(stream)} >>\nstream\n{stream}\nendstream")
    content_ids.append(c)
    page_objs.append(add(None))
pages_id = add(None)
for pid, cid in zip(page_objs, content_ids):
    objs[pid-1] = f"<< /Type /Page /Parent {pages_id} 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 {font} 0 R >> >> /Contents {cid} 0 R >>"
objs[pages_id-1] = f"<< /Type /Pages /Kids [{' '.join(f'{p} 0 R' for p in page_objs)}] /Count {pages} >>"
info = add("<< /Title (Bookman Test Book) /Author (Test Author) >>")
cat = add(f"<< /Type /Catalog /Pages {pages_id} 0 R >>")
out_b = b"%PDF-1.4\n"
offsets = []
for i, o in enumerate(objs, 1):
    offsets.append(len(out_b))
    out_b += f"{i} 0 obj\n{o}\nendobj\n".encode()
x = len(out_b)
out_b += f"xref\n0 {len(objs)+1}\n0000000000 65535 f \n".encode()
for off in offsets: out_b += f"{off:010d} 00000 n \n".encode()
out_b += f"trailer\n<< /Size {len(objs)+1} /Root {cat} 0 R /Info {info} 0 R >>\nstartxref\n{x}\n%%EOF\n".encode()
open(out, "wb").write(out_b)
