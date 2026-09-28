"""
文件 → Markdown 转换接口

供 Go 后端在上传学习资料时调用，把 docx / pdf / 图片 等格式统一转成
Markdown 文本，再进入既有的图谱生成流水线。
"""

import logging

from fastapi import APIRouter, File, HTTPException, UploadFile

from app.services.document_converter import DocumentConvertError, convert_to_markdown

router = APIRouter(prefix="/api/extract", tags=["extract"])
logger = logging.getLogger(__name__)


@router.post("/to-markdown")
async def extract_to_markdown(file: UploadFile = File(...)) -> dict:
    """
    接收上传文件，返回转换后的 Markdown。

    使用 multipart/form-data，字段名为 file。
    """
    filename = file.filename or "unnamed"
    try:
        raw = await file.read()
    except Exception as exc:  # noqa: BLE001
        logger.exception("[Extract] 读取上传文件失败")
        raise HTTPException(status_code=400, detail=f"读取上传文件失败: {exc}") from exc
    finally:
        await file.close()

    try:
        markdown, source = convert_to_markdown(filename, raw)
    except DocumentConvertError as exc:
        # 面向用户的格式/解析问题，返回 400 让前端展示可读原因
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    except Exception as exc:  # noqa: BLE001
        logger.exception("[Extract] 文档转换出现未预期错误")
        raise HTTPException(status_code=500, detail=f"文档转换失败: {exc}") from exc

    if not markdown.strip():
        raise HTTPException(status_code=400, detail="转换后内容为空，请检查文件是否有可读文字")

    return {
        "filename": filename,
        "source_type": source,
        "markdown": markdown,
        "length": len(markdown),
    }
