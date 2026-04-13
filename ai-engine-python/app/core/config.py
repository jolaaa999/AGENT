import os
from dataclasses import dataclass

from dotenv import load_dotenv


load_dotenv()


@dataclass(frozen=True)
class Settings:
    deepseek_base_url: str = os.getenv("DEEPSEEK_BASE_URL", "https://api.deepseek.com")
    deepseek_api_key: str = os.getenv("DEEPSEEK_API_KEY", "")
    deepseek_model: str = os.getenv("DEEPSEEK_MODEL", "deepseek-chat")

    def validate_or_raise(self) -> None:
        if not self.deepseek_api_key.strip():
            raise RuntimeError(
                "Missing required env var DEEPSEEK_API_KEY. "
                "Please create ai-engine-python/.env from .env.example and set it."
            )


settings = Settings()
