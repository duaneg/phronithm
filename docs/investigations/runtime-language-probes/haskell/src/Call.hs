{-# LANGUAGE OverloadedStrings #-}
module Call (Outcome (..), call, parallelCalls) where

import Control.Concurrent.Async (mapConcurrently, race)
import Control.Concurrent (threadDelay)
import Control.Exception (try, SomeException)
import Data.Aeson (Object, Value (Object, String), decodeStrict)
import qualified Data.Aeson.KeyMap as KM
import qualified Data.ByteString.Char8 as B
import System.IO (hClose)
import System.Posix.Signals (signalProcessGroup, sigKILL)
import System.Process
import System.Environment (getEnvironment)

data Outcome = Ok Object | Null | Timeout deriving (Eq, Show)

call :: FilePath -> [(String, String)] -> Int -> IO Outcome
call binary env timeoutUs = do
  baseEnv <- getEnvironment
  (_, Just out, _, ph) <- createProcess (proc binary ["-p", "x", "--output-format", "stream-json"])
    { std_out = CreatePipe, env = Just (env ++ baseEnv), create_group = True }
  r <- race (threadDelay timeoutUs) (B.hGetContents out)
  case r of
    Left () -> do
      mpid <- getPid ph
      mapM_ (\p -> try (signalProcessGroup sigKILL p) :: IO (Either SomeException ())) mpid
      hClose out
      _ <- waitForProcess ph
      pure Timeout
    Right bytes -> do
      _ <- waitForProcess ph
      pure (foldl pick Null (B.lines bytes))
  where
    pick acc line = case decodeStrict line of
      Just (Object ev)
        | KM.lookup "type" ev == Just (String "result") ->
            case KM.lookup "structured_output" ev of
              Just (Object o) -> Ok o
              _ -> Null
      _ -> acc
    

parallelCalls :: FilePath -> Int -> IO [Outcome]
parallelCalls binary n = mapConcurrently (const (call binary [("STUB_MODE", "ok")] 10000000)) [1 .. n]
